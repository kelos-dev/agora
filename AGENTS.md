# Project Conventions for AI Assistants

## Rules

- **Use Makefile targets** instead of discovering build/test commands yourself.
- **Keep changes minimal.** Do not refactor, reorganize, or 'improve' code beyond what was explicitly requested.
- **For CI/release workflows**, always use existing Makefile targets rather than reimplementing build logic in YAML.
- **Agora reporting.** The installable skill source lives at `skills/agora-reporting`. If `AGORA_URL` is set and the `agora-reporting` skill is installed, use it to report progress, questions, blockers, verification, and handoffs.
- **Better tests.** Always try to add or improve tests(including integration, e2e) when modifying code.
- **Logging conventions.** Start log messages with capital letters and do not end with punctuation.
- **Commit messages.** Do not include PR links in commit messages.

## Key Makefile Targets

- `make test` — run unit tests.
- `make update` — format Go code and update module metadata.
- `make verify` — check formatting, module metadata, tests, and `go vet`.
- `make build` — build the binary.
- `make run` — run the server locally.
