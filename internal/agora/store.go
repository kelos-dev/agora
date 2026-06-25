package agora

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	recordEvent        = "event"
	recordStatusChange = "status_change"
)

type Store struct {
	mu     sync.RWMutex
	path   string
	events []Event
	byID   map[string]int
}

type logRecord struct {
	Kind   string        `json:"kind"`
	Event  *Event        `json:"event,omitempty"`
	Status *StatusChange `json:"status,omitempty"`
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("store path is required")
	}
	store := &Store{
		path: filepath.Clean(path),
		byID: make(map[string]int),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) AppendEvent(req CreateEventRequest) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.prepareReply(&req); err != nil {
		return Event{}, err
	}
	event, err := newEvent(req, time.Now())
	if err != nil {
		return Event{}, err
	}

	if err := s.appendRecord(logRecord{Kind: recordEvent, Event: &event}); err != nil {
		return Event{}, err
	}
	s.byID[event.ID] = len(s.events)
	s.events = append(s.events, event)
	return event, nil
}

func (s *Store) prepareReply(req *CreateEventRequest) error {
	replyTo := strings.TrimSpace(req.ReplyTo)
	if replyTo == "" {
		return nil
	}
	req.ReplyTo = replyTo

	parentIndex, ok := s.byID[replyTo]
	if !ok {
		return fmt.Errorf("reply_to event %q not found", replyTo)
	}
	parentThread := s.events[parentIndex].Thread
	thread := strings.TrimSpace(req.Thread)
	if thread == "" {
		req.Thread = parentThread
		return nil
	}
	if thread != parentThread {
		return fmt.Errorf("reply thread %q does not match parent thread %q", thread, parentThread)
	}
	return nil
}

func (s *Store) UpdateStatus(id string, req StatusUpdateRequest) (Event, error) {
	status, err := validateStatus(req.Status)
	if err != nil {
		return Event{}, err
	}
	actor := normalizeTarget(req.Actor)
	if actor == "" {
		actor = "anonymous"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	index, ok := s.byID[id]
	if !ok {
		return Event{}, fmt.Errorf("event %q not found", id)
	}

	change := StatusChange{
		EventID:   id,
		Actor:     actor,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.appendRecord(logRecord{Kind: recordStatusChange, Status: &change}); err != nil {
		return Event{}, err
	}
	s.events[index].Status = status
	return s.events[index], nil
}

func (s *Store) ListEvents(filter EventFilter) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := 0
	if filter.AfterID != "" {
		if index, ok := s.byID[filter.AfterID]; ok {
			start = index + 1
		}
	}

	limit := clampLimit(filter.Limit)
	out := make([]Event, 0, min(limit, len(s.events)))
	for _, event := range s.events[start:] {
		if !eventMatches(event, filter) {
			continue
		}
		out = append(out, event)
		if len(out) == limit {
			break
		}
	}
	return slices.Clone(out)
}

func (s *Store) Inbox(agent string, filter EventFilter) []Event {
	filter.Agent = agent
	return s.ListEvents(filter)
}

func (s *Store) Threads() []ThreadSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summaries := make(map[string]ThreadSummary)
	for _, event := range s.events {
		summary := summaries[event.Thread]
		summary.Thread = event.Thread
		summary.Count++
		summary.LastEvent = event
		summary.UpdatedAt = event.CreatedAt
		if isOpenStatus(event.Status) {
			summary.OpenCount++
		}
		summaries[event.Thread] = summary
	}

	out := make([]ThreadSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Store) load() error {
	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		var record logRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("decode store line %d: %w", line, err)
		}
		if err := s.applyRecord(record); err != nil {
			return fmt.Errorf("apply store line %d: %w", line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read store: %w", err)
	}
	return nil
}

func (s *Store) applyRecord(record logRecord) error {
	switch record.Kind {
	case recordEvent:
		if record.Event == nil {
			return fmt.Errorf("event record is missing event")
		}
		event := *record.Event
		if event.ID == "" {
			return fmt.Errorf("event record is missing id")
		}
		s.byID[event.ID] = len(s.events)
		s.events = append(s.events, event)
	case recordStatusChange:
		if record.Status == nil {
			return fmt.Errorf("status record is missing status")
		}
		index, ok := s.byID[record.Status.EventID]
		if !ok {
			return fmt.Errorf("status record references unknown event %q", record.Status.EventID)
		}
		s.events[index].Status = record.Status.Status
	default:
		return fmt.Errorf("unknown record kind %q", record.Kind)
	}
	return nil
}

func (s *Store) appendRecord(record logRecord) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create store directory: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open store for append: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(record); err != nil {
		return fmt.Errorf("append store record: %w", err)
	}
	return nil
}

func eventMatches(event Event, filter EventFilter) bool {
	if filter.Thread != "" && event.Thread != filter.Thread {
		return false
	}
	if filter.ReplyTo != "" && event.ReplyTo != filter.ReplyTo {
		return false
	}
	if len(filter.Statuses) > 0 && !slices.Contains(filter.Statuses, event.Status) {
		return false
	}
	if len(filter.Statuses) == 0 && filter.Status != "" && event.Status != filter.Status {
		return false
	}
	if filter.OpenOnly && !isOpenStatus(event.Status) {
		return false
	}
	if filter.Agent != "" && !targetsAgent(event, filter.Agent) {
		return false
	}
	return true
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 100
	case limit > 500:
		return 500
	default:
		return limit
	}
}
