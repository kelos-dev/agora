package agoracli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kelos-dev/agora/internal/agora"
)

func TestPostSendsEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/events" {
			t.Fatalf("path = %s, want /api/events", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		var req agora.CreateEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Type != agora.TypeSummary {
			t.Fatalf("type = %q, want %q", req.Type, agora.TypeSummary)
		}
		if req.Actor != "codex" {
			t.Fatalf("actor = %q, want codex", req.Actor)
		}
		if req.Thread != "general" {
			t.Fatalf("thread = %q, want general", req.Thread)
		}
		if req.Title != "Started task" {
			t.Fatalf("title = %q, want Started task", req.Title)
		}
		if got := strings.Join(req.Targets, ","); got != "human,codex" {
			t.Fatalf("targets = %q, want human,codex", got)
		}
		if req.Links["pr"] != "https://example.test/pr/1" {
			t.Fatalf("pr link = %q, want https://example.test/pr/1", req.Links["pr"])
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(agora.Event{
			ID:     "evt-1",
			Type:   req.Type,
			Thread: req.Thread,
		})
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"post",
		"--type", "summary",
		"--title", "Started task",
		"--target", "human,@codex",
		"--target", "codex",
		"--link", "pr=https://example.test/pr/1",
	}, Config{
		BaseURL:       server.URL,
		Token:         "test-token",
		DefaultActor:  "codex",
		DefaultThread: "general",
		Stdout:        &stdout,
		Stderr:        &stderr,
		HTTPClient:    server.Client(),
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if got := stdout.String(); got != "posted evt-1 summary #general\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestPostReplyOmitsDefaultThread(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agora.CreateEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.ReplyTo != "evt-parent" {
			t.Fatalf("reply_to = %q, want evt-parent", req.ReplyTo)
		}
		if req.Thread != "" {
			t.Fatalf("thread = %q, want empty", req.Thread)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(agora.Event{
			ID:      "evt-reply",
			Type:    req.Type,
			Thread:  "general",
			ReplyTo: req.ReplyTo,
		})
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"post",
		"--type", "comment",
		"--title", "Reply",
		"--reply-to", "evt-parent",
	}, Config{
		BaseURL:       server.URL,
		DefaultActor:  "codex",
		DefaultThread: "general",
		Stdout:        &stdout,
		Stderr:        &stderr,
		HTTPClient:    server.Client(),
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if got := stdout.String(); got != "posted evt-reply comment #general\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestSessionPrintsShellExportWithoutURL(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"session",
		"--repo", "git@github.com:kelos-dev/agora.git",
		"--actor", "codex-one",
		"--task", "Fix #42",
	}, Config{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if got := stdout.String(); !regexp.MustCompile(`^export AGORA_THREAD=agora-codex-one-fix-42-\d{8}T\d{6}Z-[0-9a-f]{6}\n$`).MatchString(got) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestSessionPrintsValueFormat(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"session",
		"--repo", "agora",
		"--actor", "codex",
		"--format", "value",
	}, Config{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if got := stdout.String(); !regexp.MustCompile(`^agora-codex-\d{8}T\d{6}Z-[0-9a-f]{6}\n$`).MatchString(got) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestSessionThreadUsesStableParts(t *testing.T) {
	now := time.Date(2026, 6, 25, 22, 35, 0, 0, time.UTC)

	got := sessionThread("git@github.com:kelos-dev/agora.git", "Codex One", "Fix #42", now, "abcdef")
	want := "agora-codex-one-fix-42-20260625T223500Z-abcdef"
	if got != want {
		t.Fatalf("sessionThread() = %q, want %q", got, want)
	}
}

func TestInboxShowsActionableEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/agents/codex/inbox" {
			t.Fatalf("path = %s, want /api/agents/codex/inbox", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("limit = %q, want 20", r.URL.Query().Get("limit"))
		}
		if got := strings.Join(r.URL.Query()["status"], ","); got != "acknowledged,open,posted" {
			t.Fatalf("status filters = %q, want acknowledged,open,posted", got)
		}
		if got := r.URL.Query().Get("open"); got != "" {
			t.Fatalf("open filter = %q, want empty", got)
		}
		_ = json.NewEncoder(w).Encode([]agora.Event{
			{ID: "evt-1", Status: agora.StatusPosted, Type: agora.TypeInstruction, Actor: "human", Thread: "general", Title: "Do this"},
			{ID: "evt-2", Status: agora.StatusResolved, Type: agora.TypeComment, Actor: "human", Thread: "general", Title: "Done"},
			{ID: "evt-3", Status: agora.StatusAcknowledged, Type: agora.TypeQuestion, Actor: "human", Thread: "api", Body: "Choose one"},
		})
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"inbox"}, Config{
		BaseURL:      server.URL,
		DefaultActor: "codex",
		Stdout:       &stdout,
		Stderr:       &stderr,
		HTTPClient:   server.Client(),
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "evt-1 [posted] instruction from @human #general: Do this") {
		t.Fatalf("stdout missing posted event: %q", got)
	}
	if strings.Contains(got, "evt-2") {
		t.Fatalf("stdout includes resolved event: %q", got)
	}
	if !strings.Contains(got, "evt-3 [acknowledged] question from @human #api: Choose one\n  Choose one") {
		t.Fatalf("stdout missing acknowledged event body: %q", got)
	}
}

func TestInboxOpenSendsServerFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("open"); got != "true" {
			t.Fatalf("open filter = %q, want true", got)
		}
		if got := r.URL.Query()["status"]; len(got) != 0 {
			t.Fatalf("status filters = %#v, want none", got)
		}
		_ = json.NewEncoder(w).Encode([]agora.Event{
			{ID: "evt-1", Status: agora.StatusOpen, Type: agora.TypeInstruction, Actor: "human", Thread: "general", Title: "Do this"},
		})
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"inbox", "--open"}, Config{
		BaseURL:      server.URL,
		DefaultActor: "codex",
		Stdout:       &stdout,
		Stderr:       &stderr,
		HTTPClient:   server.Client(),
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "evt-1 [open] instruction from @human #general: Do this") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestStatusUpdatesEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/events/evt-1/status" {
			t.Fatalf("path = %s, want /api/events/evt-1/status", r.URL.Path)
		}

		var req agora.StatusUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Actor != "codex" {
			t.Fatalf("actor = %q, want codex", req.Actor)
		}
		if req.Status != agora.StatusAcknowledged {
			t.Fatalf("status = %q, want acknowledged", req.Status)
		}
		_ = json.NewEncoder(w).Encode(agora.Event{ID: "evt-1", Status: req.Status})
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"status", "evt-1", "acknowledged"}, Config{
		BaseURL:      server.URL,
		DefaultActor: "codex",
		Stdout:       &stdout,
		Stderr:       &stderr,
		HTTPClient:   server.Client(),
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if got := stdout.String(); got != "evt-1 status=acknowledged\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestMissingURLSkipsRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"post", "--type", "summary", "--title", "Started task"}, Config{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); got != "AGORA_URL is not set; skipping Agora.\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestNormalizeBaseURLAcceptsSchemeLessHostPort(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:8080":           "http://127.0.0.1:8080",
		"localhost:8080/":          "http://localhost:8080",
		"https://example.test/api": "https://example.test/api",
	}

	for input, want := range tests {
		got, err := normalizeBaseURL(input)
		if err != nil {
			t.Fatalf("normalizeBaseURL(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}
