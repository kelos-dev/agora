package agora

import (
	"path/filepath"
	"testing"
)

func TestStorePersistsEventsAndStatusChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	event, err := store.AppendEvent(CreateEventRequest{
		Type:    TypeInstruction,
		Actor:   "human",
		Thread:  "api-design",
		Title:   "Use option A",
		Targets: []string{"agent-one"},
	})
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if event.Status != StatusOpen {
		t.Fatalf("event status = %q, want %q", event.Status, StatusOpen)
	}

	updated, err := store.UpdateStatus(event.ID, StatusUpdateRequest{
		Actor:  "agent-one",
		Status: StatusAcknowledged,
	})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	if updated.Status != StatusAcknowledged {
		t.Fatalf("updated status = %q, want %q", updated.Status, StatusAcknowledged)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() reload error = %v", err)
	}
	events := reloaded.ListEvents(EventFilter{})
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Status != StatusAcknowledged {
		t.Fatalf("reloaded status = %q, want %q", events[0].Status, StatusAcknowledged)
	}
}

func TestInboxMatchesTargetsAndMentions(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	_, err = store.AppendEvent(CreateEventRequest{
		Type:   TypeQuestion,
		Actor:  "human",
		Thread: "release",
		Title:  "Can @builder run the release checks?",
	})
	if err != nil {
		t.Fatalf("AppendEvent() mention error = %v", err)
	}
	_, err = store.AppendEvent(CreateEventRequest{
		Type:    TypeSummary,
		Actor:   "tester",
		Thread:  "release",
		Title:   "Tests passed",
		Targets: []string{"all"},
	})
	if err != nil {
		t.Fatalf("AppendEvent() all error = %v", err)
	}

	inbox := store.Inbox("builder", EventFilter{})
	if len(inbox) != 2 {
		t.Fatalf("len(inbox) = %d, want 2", len(inbox))
	}
	if inbox[0].Type != TypeQuestion {
		t.Fatalf("first inbox type = %q, want %q", inbox[0].Type, TypeQuestion)
	}
}
