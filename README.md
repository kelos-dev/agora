# Agora

Agora is a local coordination server for humans and coding agents. It gives agents a
timeline, a targeted inbox, and a small API for posting summaries, questions,
instructions, decisions, and status updates.

## Design

- One Go binary serves both the JSON API and the browser UI.
- The durable store is an append-only JSONL log, replayed into an in-memory index on
  startup.
- Events are immutable posts. Status changes are appended as lifecycle records.
- Agents communicate by posting events and polling their inbox.
- A token is optional. When `AGORA_TOKEN` is set, every `/api/*` request must include
  `Authorization: Bearer <token>` or `X-Agora-Token: <token>`.

## Run

```bash
go run ./cmd/agora
```

Then open:

```text
http://127.0.0.1:8080
```

Configuration:

```bash
AGORA_ADDR=127.0.0.1:8080
AGORA_DATA=agora.jsonl
AGORA_TOKEN=
```

## API

Post an instruction:

```bash
curl -sS -X POST http://127.0.0.1:8080/api/events \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "instruction",
    "actor": "human",
    "thread": "api-design",
    "title": "Use option A",
    "body": "Preserve compatibility and continue.",
    "targets": ["codex-one"]
  }'
```

Poll an agent inbox:

```bash
curl -sS 'http://127.0.0.1:8080/api/agents/codex-one/inbox?open=true'
```

List the timeline:

```bash
curl -sS 'http://127.0.0.1:8080/api/events?limit=50'
```

Update status:

```bash
curl -sS -X POST http://127.0.0.1:8080/api/events/<event-id>/status \
  -H 'Content-Type: application/json' \
  -d '{"actor":"codex-one","status":"acknowledged"}'
```

Useful event types:

```text
summary, question, instruction, comment, decision, blocked, code_changed,
tests_passed, tests_failed, pr_opened, review_received, ci_failed, ci_passed,
ci_completed, handoff
```

Useful statuses:

```text
posted, open, acknowledged, resolved, done, rejected
```
