package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kelos-dev/agora/internal/agora"
)

func TestEventAPIAndInbox(t *testing.T) {
	store, err := agora.NewStore(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	app, err := New(store, Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body := bytes.NewBufferString(`{
		"type": "instruction",
		"actor": "human",
		"thread": "api",
		"title": "Continue with option A",
		"targets": ["codex-one"]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/events", body)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/events status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var event agora.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agents/codex-one/inbox?open=true", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET inbox status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var inbox []agora.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &inbox); err != nil {
		t.Fatalf("decode inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].ID != event.ID {
		t.Fatalf("inbox = %#v, want event %s", inbox, event.ID)
	}

	replyBody, err := json.Marshal(map[string]string{
		"type":     "comment",
		"actor":    "codex-one",
		"body":     "I will continue there.",
		"reply_to": event.ID,
	})
	if err != nil {
		t.Fatalf("encode reply body: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(replyBody))
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST reply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var reply agora.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.ReplyTo != event.ID {
		t.Fatalf("reply ReplyTo = %q, want %q", reply.ReplyTo, event.ID)
	}
	if reply.Thread != event.Thread {
		t.Fatalf("reply Thread = %q, want %q", reply.Thread, event.Thread)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/events?reply_to="+event.ID, nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET replies status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var replies []agora.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &replies); err != nil {
		t.Fatalf("decode replies: %v", err)
	}
	if len(replies) != 1 || replies[0].ID != reply.ID {
		t.Fatalf("replies = %#v, want reply %s", replies, reply.ID)
	}
}
