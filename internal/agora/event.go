package agora

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	TypeBlocked     = "blocked"
	TypeCICompleted = "ci_completed"
	TypeCIFailed    = "ci_failed"
	TypeCIPassed    = "ci_passed"
	TypeCodeChanged = "code_changed"
	TypeComment     = "comment"
	TypeDecision    = "decision"
	TypeHandoff     = "handoff"
	TypeInstruction = "instruction"
	TypePROpened    = "pr_opened"
	TypeQuestion    = "question"
	TypeReview      = "review_received"
	TypeSummary     = "summary"
	TypeTestsFailed = "tests_failed"
	TypeTestsPassed = "tests_passed"
)

const (
	SeverityBlocked       = "blocked"
	SeverityInfo          = "info"
	SeverityNeedsDecision = "needs_decision"
	SeverityAttention     = "attention"
)

const (
	StatusAcknowledged = "acknowledged"
	StatusDone         = "done"
	StatusOpen         = "open"
	StatusPosted       = "posted"
	StatusRejected     = "rejected"
	StatusResolved     = "resolved"
)

var (
	allowedEventTypes = map[string]bool{
		TypeBlocked:     true,
		TypeCICompleted: true,
		TypeCIFailed:    true,
		TypeCIPassed:    true,
		TypeCodeChanged: true,
		TypeComment:     true,
		TypeDecision:    true,
		TypeHandoff:     true,
		TypeInstruction: true,
		TypePROpened:    true,
		TypeQuestion:    true,
		TypeReview:      true,
		TypeSummary:     true,
		TypeTestsFailed: true,
		TypeTestsPassed: true,
	}
	allowedSeverities = map[string]bool{
		SeverityBlocked:       true,
		SeverityInfo:          true,
		SeverityNeedsDecision: true,
		SeverityAttention:     true,
	}
	allowedStatuses = map[string]bool{
		StatusAcknowledged: true,
		StatusDone:         true,
		StatusOpen:         true,
		StatusPosted:       true,
		StatusRejected:     true,
		StatusResolved:     true,
	}
	mentionPattern = regexp.MustCompile(`(?i)(^|[^\w])@([a-z0-9._-]+)`)
	targetPattern  = regexp.MustCompile(`[^a-z0-9._-]+`)
)

type Event struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"created_at"`
	Type      string            `json:"type"`
	Actor     string            `json:"actor"`
	Thread    string            `json:"thread"`
	Title     string            `json:"title,omitempty"`
	Body      string            `json:"body,omitempty"`
	Targets   []string          `json:"targets,omitempty"`
	Repo      string            `json:"repo,omitempty"`
	Task      string            `json:"task,omitempty"`
	Severity  string            `json:"severity"`
	Status    string            `json:"status"`
	ReplyTo   string            `json:"reply_to,omitempty"`
	Links     map[string]string `json:"links,omitempty"`
}

type CreateEventRequest struct {
	Type     string            `json:"type"`
	Actor    string            `json:"actor"`
	Thread   string            `json:"thread"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Targets  []string          `json:"targets"`
	Repo     string            `json:"repo"`
	Task     string            `json:"task"`
	Severity string            `json:"severity"`
	Status   string            `json:"status"`
	ReplyTo  string            `json:"reply_to"`
	Links    map[string]string `json:"links"`
}

type StatusUpdateRequest struct {
	Actor  string `json:"actor"`
	Status string `json:"status"`
}

type StatusChange struct {
	EventID   string    `json:"event_id"`
	Actor     string    `json:"actor"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type EventFilter struct {
	AfterID  string
	Agent    string
	Limit    int
	OpenOnly bool
	Status   string
	Thread   string
}

type ThreadSummary struct {
	Thread    string    `json:"thread"`
	Count     int       `json:"count"`
	OpenCount int       `json:"open_count"`
	LastEvent Event     `json:"last_event"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newEvent(req CreateEventRequest, now time.Time) (Event, error) {
	eventType := normalizeChoice(req.Type)
	if eventType == "" {
		eventType = TypeSummary
	}
	if !allowedEventTypes[eventType] {
		return Event{}, fmt.Errorf("unknown event type %q", req.Type)
	}

	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Body)
	if title == "" && body == "" {
		return Event{}, fmt.Errorf("title or body is required")
	}

	severity := normalizeChoice(req.Severity)
	if severity == "" {
		severity = defaultSeverity(eventType)
	}
	if !allowedSeverities[severity] {
		return Event{}, fmt.Errorf("unknown severity %q", req.Severity)
	}

	status := normalizeChoice(req.Status)
	if status == "" {
		status = defaultStatus(eventType)
	}
	if !allowedStatuses[status] {
		return Event{}, fmt.Errorf("unknown status %q", req.Status)
	}

	thread := strings.TrimSpace(req.Thread)
	if thread == "" {
		thread = "general"
	}

	actor := normalizeTarget(req.Actor)
	if actor == "" {
		actor = "anonymous"
	}

	return Event{
		ID:        newID(now),
		CreatedAt: now.UTC(),
		Type:      eventType,
		Actor:     actor,
		Thread:    thread,
		Title:     title,
		Body:      body,
		Targets:   mergeTargets(req.Targets, extractMentions(title), extractMentions(body)),
		Repo:      strings.TrimSpace(req.Repo),
		Task:      strings.TrimSpace(req.Task),
		Severity:  severity,
		Status:    status,
		ReplyTo:   strings.TrimSpace(req.ReplyTo),
		Links:     cleanLinks(req.Links),
	}, nil
}

func validateStatus(status string) (string, error) {
	normalized := normalizeChoice(status)
	if normalized == "" {
		return "", fmt.Errorf("status is required")
	}
	if !allowedStatuses[normalized] {
		return "", fmt.Errorf("unknown status %q", status)
	}
	return normalized, nil
}

func targetsAgent(event Event, agent string) bool {
	agent = normalizeTarget(agent)
	if agent == "" {
		return false
	}
	for _, target := range event.Targets {
		if target == agent || target == "all" {
			return true
		}
	}
	return false
}

func isOpenStatus(status string) bool {
	return status == StatusOpen || status == StatusAcknowledged
}

func defaultSeverity(eventType string) string {
	switch eventType {
	case TypeBlocked:
		return SeverityBlocked
	case TypeInstruction, TypeQuestion:
		return SeverityNeedsDecision
	case TypeCIFailed, TypeTestsFailed:
		return SeverityAttention
	default:
		return SeverityInfo
	}
}

func defaultStatus(eventType string) string {
	switch eventType {
	case TypeBlocked, TypeInstruction, TypeQuestion:
		return StatusOpen
	default:
		return StatusPosted
	}
}

func newID(now time.Time) string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("evt_%s", now.UTC().Format("20060102T150405000000000"))
	}
	return fmt.Sprintf("evt_%s_%s", now.UTC().Format("20060102T150405000000000"), hex.EncodeToString(buf[:]))
}

func normalizeChoice(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeTarget(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	value = strings.ToLower(value)
	value = targetPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-._")
}

func extractMentions(value string) []string {
	matches := mentionPattern.FindAllStringSubmatch(value, -1)
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		target := normalizeTarget(match[2])
		if target != "" {
			targets = append(targets, target)
		}
	}
	return targets
}

func mergeTargets(groups ...[]string) []string {
	targets := make([]string, 0)
	for _, group := range groups {
		for _, target := range group {
			target = normalizeTarget(target)
			if target == "" || slices.Contains(targets, target) {
				continue
			}
			targets = append(targets, target)
		}
	}
	return targets
}

func cleanLinks(links map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range links {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
