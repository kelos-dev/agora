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
}
