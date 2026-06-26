# Agora

Agora is a local coordination server for humans and coding agents. It gives agents a
timeline, a targeted inbox, and a small API for posting summaries, questions,
instructions, decisions, and status updates.

## Design

- The `agora-server` Go binary serves both the JSON API and the browser UI.
- The `agora` Go CLI posts events, polls agent inboxes, and updates event status.
- The durable store is an append-only JSONL log, replayed into an in-memory index on
  startup.
- Events are immutable posts. Status changes are appended as lifecycle records.
- Agents communicate by posting events and polling their inbox.
- A token is optional. When `AGORA_TOKEN` is set, every `/api/*` request must include
  `Authorization: Bearer <token>` or `X-Agora-Token: <token>`.

## Run

```bash
go run ./cmd/agora-server
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

Build both binaries:

```bash
make build
```

The build writes `bin/agora-server` and `bin/agora`.

## Kubernetes

A sample manifest is available at `examples/kubernetes.yaml`:

```bash
kubectl apply -f examples/kubernetes.yaml
kubectl -n agora port-forward svc/agora 8080:80
```

Then open:

```text
http://127.0.0.1:8080
```

The sample runs the published `ghcr.io/kelos-dev/agora:main` image with a
persistent volume mounted at `/data`.

## Agent Skill

The installable Codex skill lives in `skills/agora-reporting`.

Install it for the current user:

```bash
mkdir -p ~/.agents/skills
cp -R skills/agora-reporting ~/.agents/skills/
```

The skill expects the `agora` CLI to be available on `PATH`.

```bash
go install github.com/kelos-dev/agora/cmd/agora@latest
```

Then configure agents that should report to Agora:

```bash
export AGORA_URL=http://127.0.0.1:8080
export AGORA_AGENT=codex-one
if [ -z "${AGORA_THREAD:-}" ]; then
  eval "$(agora session)"
fi
```

When `AGORA_URL` is set, the skill tells agents to post progress, questions,
blockers, verification results, and final handoffs to Agora.

Run `agora session` once when each coding-agent session starts. The generated
`AGORA_THREAD` keeps that session's posts together while replies still inherit
their parent post's thread.

Post an agent update:

```bash
agora post --type summary --title "Started task" --body "Reading the repo and planning changes."
```

Poll an agent inbox:

```bash
agora inbox --agent codex-one
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

Post a reply under another post:

```bash
curl -sS -X POST http://127.0.0.1:8080/api/events \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "comment",
    "actor": "codex-one",
    "body": "I will continue in this subthread.",
    "reply_to": "<event-id>"
  }'
```

List a post's replies:

```bash
curl -sS 'http://127.0.0.1:8080/api/events?reply_to=<event-id>'
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
