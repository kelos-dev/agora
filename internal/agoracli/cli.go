package agoracli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kelos-dev/agora/internal/agora"
)

type Config struct {
	BaseURL       string
	Token         string
	DefaultActor  string
	DefaultRepo   string
	DefaultTask   string
	DefaultThread string
	HTTPClient    *http.Client
	Stdout        io.Writer
	Stderr        io.Writer
}

var eventTypes = map[string]bool{
	agora.TypeBlocked:     true,
	agora.TypeCICompleted: true,
	agora.TypeCIFailed:    true,
	agora.TypeCIPassed:    true,
	agora.TypeCodeChanged: true,
	agora.TypeComment:     true,
	agora.TypeDecision:    true,
	agora.TypeHandoff:     true,
	agora.TypeInstruction: true,
	agora.TypePROpened:    true,
	agora.TypeQuestion:    true,
	agora.TypeReview:      true,
	agora.TypeSummary:     true,
	agora.TypeTestsFailed: true,
	agora.TypeTestsPassed: true,
}

var statuses = map[string]bool{
	agora.StatusAcknowledged: true,
	agora.StatusDone:         true,
	agora.StatusOpen:         true,
	agora.StatusPosted:       true,
	agora.StatusRejected:     true,
	agora.StatusResolved:     true,
}

var actionableStatuses = map[string]bool{
	agora.StatusAcknowledged: true,
	agora.StatusOpen:         true,
	agora.StatusPosted:       true,
}

var actionableStatusList = []string{
	agora.StatusAcknowledged,
	agora.StatusOpen,
	agora.StatusPosted,
}

func Run(ctx context.Context, args []string, cfg Config) int {
	cfg = cfg.withDefaults()

	if len(args) == 0 {
		printUsage(cfg.stderr())
		return 2
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage(cfg.stdout())
		return 0
	case "post":
		return runPost(ctx, args[1:], cfg)
	case "inbox":
		return runInbox(ctx, args[1:], cfg)
	case "status":
		return runStatus(ctx, args[1:], cfg)
	default:
		fmt.Fprintf(cfg.stderr(), "agora: unknown command %q\n\n", args[0])
		printUsage(cfg.stderr())
		return 2
	}
}

func runPost(ctx context.Context, args []string, cfg Config) int {
	var targets repeatFlag
	var links repeatFlag
	thread := optionalStringFlag{value: cfg.DefaultThread}
	fs := newFlagSet("agora post", cfg)
	eventType := fs.String("type", "", "event type")
	title := fs.String("title", "", "event title")
	body := fs.String("body", "", "event body")
	actor := fs.String("actor", cfg.DefaultActor, "actor name")
	repo := fs.String("repo", cfg.DefaultRepo, "repository name or URL")
	task := fs.String("task", cfg.DefaultTask, "task identifier")
	severity := fs.String("severity", "", "event severity")
	replyTo := fs.String("reply-to", "", "parent event id")
	fs.Var(&thread, "thread", "thread name")
	fs.Var(&targets, "target", "target agent, repeatable or comma-separated")
	fs.Var(&links, "link", "link in NAME=URL form, repeatable")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: agora post --type TYPE --title TITLE [flags]")
		fs.PrintDefaults()
	}
	if code := parseFlags(fs, args); code != -1 {
		return code
	}
	if *eventType == "" {
		return usageError(fs, "--type is required")
	}
	if !eventTypes[*eventType] {
		return usageError(fs, "unknown event type %q", *eventType)
	}
	if strings.TrimSpace(*title) == "" {
		return usageError(fs, "--title is required")
	}
	if skipIfNoServer(cfg) {
		return 0
	}

	threadValue := thread.value
	if !thread.set && *replyTo != "" {
		threadValue = ""
	}
	payload := compact(map[string]any{
		"type":     *eventType,
		"actor":    *actor,
		"thread":   threadValue,
		"title":    *title,
		"body":     *body,
		"targets":  splitTargets(targets),
		"repo":     *repo,
		"task":     *task,
		"severity": *severity,
		"reply_to": *replyTo,
		"links":    parseLinks(links),
	})

	var event agora.Event
	if err := requestJSON(ctx, cfg, http.MethodPost, "/api/events", payload, &event); err != nil {
		fmt.Fprintln(cfg.stderr(), err)
		return 1
	}
	fmt.Fprintf(cfg.stdout(), "posted %s %s #%s\n", event.ID, event.Type, event.Thread)
	return 0
}

func runInbox(ctx context.Context, args []string, cfg Config) int {
	fs := newFlagSet("agora inbox", cfg)
	agent := fs.String("agent", cfg.DefaultActor, "agent name")
	all := fs.Bool("all", false, "show closed and already handled events too")
	open := fs.Bool("open", false, "only show open and acknowledged events")
	limit := fs.Int("limit", 20, "maximum number of events to fetch")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: agora inbox [flags]")
		fs.PrintDefaults()
	}
	if code := parseFlags(fs, args); code != -1 {
		return code
	}
	if strings.TrimSpace(*agent) == "" {
		return usageError(fs, "--agent is required")
	}
	if skipIfNoServer(cfg) {
		return 0
	}

	query := url.Values{"limit": {fmt.Sprint(*limit)}}
	switch {
	case *all:
	case *open:
		query.Set("open", "true")
	default:
		for _, status := range actionableStatusList {
			query.Add("status", status)
		}
	}
	apiPath := fmt.Sprintf("/api/agents/%s/inbox?%s", url.PathEscape(*agent), query.Encode())
	var events []agora.Event
	if err := requestJSON(ctx, cfg, http.MethodGet, apiPath, nil, &events); err != nil {
		fmt.Fprintln(cfg.stderr(), err)
		return 1
	}
	events = filterInbox(events, *all, *open)
	if len(events) == 0 {
		fmt.Fprintln(cfg.stdout(), "inbox empty")
		return 0
	}
	for _, event := range events {
		fmt.Fprintln(cfg.stdout(), renderEvent(event))
	}
	return 0
}

func runStatus(ctx context.Context, args []string, cfg Config) int {
	fs := newFlagSet("agora status", cfg)
	actor := fs.String("actor", cfg.DefaultActor, "actor name")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: agora status [flags] EVENT_ID STATUS")
		fs.PrintDefaults()
	}
	if code := parseFlags(fs, args); code != -1 {
		return code
	}
	if fs.NArg() != 2 {
		return usageError(fs, "event id and status are required")
	}
	eventID := fs.Arg(0)
	status := fs.Arg(1)
	if !statuses[status] {
		return usageError(fs, "unknown status %q", status)
	}
	if skipIfNoServer(cfg) {
		return 0
	}

	payload := map[string]any{"actor": *actor, "status": status}
	apiPath := fmt.Sprintf("/api/events/%s/status", url.PathEscape(eventID))
	var event agora.Event
	if err := requestJSON(ctx, cfg, http.MethodPost, apiPath, payload, &event); err != nil {
		fmt.Fprintln(cfg.stderr(), err)
		return 1
	}
	fmt.Fprintf(cfg.stdout(), "%s status=%s\n", event.ID, event.Status)
	return 0
}

func newFlagSet(name string, cfg Config) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(cfg.stderr())
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) int {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	return -1
}

func usageError(fs *flag.FlagSet, format string, args ...any) int {
	fmt.Fprintf(fs.Output(), "%s: %s\n\n", fs.Name(), fmt.Sprintf(format, args...))
	fs.Usage()
	return 2
}

func skipIfNoServer(cfg Config) bool {
	if strings.TrimSpace(cfg.BaseURL) != "" {
		return false
	}
	fmt.Fprintln(cfg.stderr(), "AGORA_URL is not set; skipping Agora.")
	return true
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  agora post --type TYPE --title TITLE [flags]
  agora inbox [flags]
  agora status [flags] EVENT_ID STATUS`)
}

func requestJSON(ctx context.Context, cfg Config, method, apiPath string, payload any, out any) error {
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(payload); err != nil {
			return fmt.Errorf("encode Agora request: %w", err)
		}
		body = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+apiPath, body)
	if err != nil {
		return fmt.Errorf("create Agora request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	resp, err := cfg.client().Do(req)
	if err != nil {
		return fmt.Errorf("Agora request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("Agora request failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode Agora response: %w", err)
	}
	return nil
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", fmt.Errorf("AGORA_URL is required")
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse AGORA_URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("AGORA_URL must include a host")
	}
	return value, nil
}

func filterInbox(events []agora.Event, all bool, open bool) []agora.Event {
	if all {
		return events
	}
	filtered := events[:0]
	for _, event := range events {
		switch {
		case open && (event.Status == agora.StatusAcknowledged || event.Status == agora.StatusOpen):
			filtered = append(filtered, event)
		case !open && actionableStatuses[event.Status]:
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func splitTargets(values []string) []string {
	var targets []string
	seen := map[string]bool{}
	for _, value := range values {
		for _, item := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
			item = strings.TrimPrefix(strings.TrimSpace(item), "@")
			if item != "" && !seen[item] {
				targets = append(targets, item)
				seen[item] = true
			}
		}
	}
	return targets
}

func parseLinks(values []string) map[string]string {
	links := map[string]string{}
	for _, value := range values {
		name, link, ok := strings.Cut(value, "=")
		if ok && strings.TrimSpace(name) != "" && strings.TrimSpace(link) != "" {
			links[strings.TrimSpace(name)] = strings.TrimSpace(link)
		}
	}
	return links
}

func compact(payload map[string]any) map[string]any {
	compacted := map[string]any{}
	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				compacted[key] = typed
			}
		case []string:
			if len(typed) > 0 {
				compacted[key] = typed
			}
		case map[string]string:
			if len(typed) > 0 {
				compacted[key] = typed
			}
		default:
			compacted[key] = typed
		}
	}
	return compacted
}

func renderEvent(event agora.Event) string {
	title := event.Title
	if title == "" {
		title = event.Body
	}
	if title == "" {
		title = event.Type
	}
	rendered := fmt.Sprintf(
		"%s [%s] %s from @%s #%s: %s",
		event.ID,
		event.Status,
		event.Type,
		event.Actor,
		event.Thread,
		title,
	)
	if event.Body != "" {
		rendered += "\n  " + event.Body
	}
	return rendered
}

type repeatFlag []string

func (f *repeatFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type optionalStringFlag struct {
	value string
	set   bool
}

func (f *optionalStringFlag) String() string {
	return f.value
}

func (f *optionalStringFlag) Set(value string) error {
	f.value = value
	f.set = true
	return nil
}

func (cfg Config) withDefaults() Config {
	if cfg.DefaultActor == "" {
		cfg.DefaultActor = "agent"
	}
	if cfg.DefaultThread == "" {
		cfg.DefaultThread = "general"
	}
	return cfg
}

func (cfg Config) client() *http.Client {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (cfg Config) stdout() io.Writer {
	if cfg.Stdout != nil {
		return cfg.Stdout
	}
	return io.Discard
}

func (cfg Config) stderr() io.Writer {
	if cfg.Stderr != nil {
		return cfg.Stderr
	}
	return io.Discard
}
