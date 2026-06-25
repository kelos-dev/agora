package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/kelos-dev/agora/internal/agora"
)

//go:embed static/*
var staticFiles embed.FS

type Config struct {
	Token string
}

type Server struct {
	store  *agora.Store
	token  string
	static http.Handler
}

func New(store *agora.Store, cfg Config) (*Server, error) {
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}
	return &Server{
		store:  store,
		token:  cfg.Token,
		static: http.FileServer(http.FS(sub)),
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if !s.authorized(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		s.serveAPI(w, r)
		return
	}
	s.serveStatic(w, r)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/healthz" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.URL.Path == "/api/events":
		s.handleEvents(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/events/") && strings.HasSuffix(r.URL.Path, "/status"):
		s.handleStatus(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/agents/") && strings.HasSuffix(r.URL.Path, "/inbox"):
		s.handleInbox(w, r)
	case r.URL.Path == "/api/threads" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.Threads())
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.URL.Path == "/" {
		serveEmbeddedFile(w, r, "index.html")
		return
	}
	cleaned := path.Clean(r.URL.Path)
	if cleaned == "." || strings.HasPrefix(cleaned, "/..") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	r.URL.Path = cleaned
	s.static.ServeHTTP(w, r)
}

func serveEmbeddedFile(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(staticFiles, path.Join("static", name))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListEvents(filterFromQuery(r)))
	case http.MethodPost:
		var req agora.CreateEventRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		event, err := s.store.AppendEvent(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, event)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	agent := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agent = strings.TrimSuffix(agent, "/inbox")
	agent = strings.Trim(agent, "/")
	if agent == "" {
		writeError(w, http.StatusBadRequest, "agent is required")
		return
	}
	writeJSON(w, http.StatusOK, s.store.Inbox(agent, filterFromQuery(r)))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/events/")
	id = strings.TrimSuffix(id, "/status")
	id = strings.Trim(id, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "event id is required")
		return
	}
	var req agora.StatusUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, err := s.store.UpdateStatus(id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	if r.Header.Get("X-Agora-Token") == s.token {
		return true
	}
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(auth, prefix) && strings.TrimPrefix(auth, prefix) == s.token
}

func filterFromQuery(r *http.Request) agora.EventFilter {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	return agora.EventFilter{
		AfterID:  query.Get("after"),
		Agent:    query.Get("agent"),
		Limit:    limit,
		OpenOnly: parseBool(query.Get("open")),
		Status:   query.Get("status"),
		Thread:   query.Get("thread"),
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func readJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode json: request body must contain one json value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
