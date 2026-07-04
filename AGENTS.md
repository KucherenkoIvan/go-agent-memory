# Agent instructions

agmem — harness-agnostic memory for AI agents, built on [go-kernel](https://github.com/KucherenkoIvan/go-kernel). The kernel's [architecture docs](https://github.com/KucherenkoIvan/go-kernel/blob/master/docs/architecture/1-service-structure.md) and [DESIGN.md](DESIGN.md) are the source of truth — read DESIGN.md before restructuring anything.

## Hard rules

1. **Layer discipline** (lint-enforced where possible):
   - `domain/` imports only the stdlib and `go-kernel/ddd`.
   - `application/` imports domain and ports — never `adapters/` or `shared/infra/`.
   - Transport frameworks stay inside their adapters (cobra in `adapters/cli/`, the MCP SDK in `adapters/mcp/`, tea in a future `adapters/tui/`); use-cases take `context.Context` + typed arguments.
   - Every face depends on the `memories.Service` facade, never on use-case structs directly — that is what makes the hosted (remote) mode a drop-in.
2. **Memory semantics are invariants, not conventions**: agents supersede, never edit; required one-line summary; ≥1 normalized keyword; validated kind; non-negative TTL. They live in the `Memory` aggregate — never re-checked in adapters.
3. **Two documented deviations — do not "fix" them**: FTS5 is maintained by SQL triggers in the migration (the DB is used by multiple processes over time; in-process events cannot index foreign writes), and `storage.Open` takes a flock so only one process touches the file at a time.
4. **The CLI/MCP agent contract is frozen surface**: JSON on non-TTY stdout, exit 0/1 with machine-readable errors, no prompts ever. Changing output shapes breaks other agents — treat like a public API.
5. **stdout is sacred in `cmd/agmem`**: it carries command output and MCP's stdio transport; slog stays discarded (or goes to a file if ever needed). A stray print corrupts the MCP session.
6. **Schema changes are migrations** in `internal/shared/infra/storage/migrations/` — numbered, up-only; remember the FTS triggers when altering `memories` columns.
7. **There is no CI — you are the CI.** `make lint` and `make test` must pass before every commit. Tests use real components (`:memory:` sqlite through `storage.Open`); port fakes are hand-written, no mock frameworks.

## Conventions

- Ranking weights live in one place (`adapters/sqlite/memory_reader.go` constants) — tune there, nowhere else.
- Keyword conventions: lowercase; `project:<name>` / `task:<name>` prefixes for scoping. `domain.NormalizeKeywords` is the single normalizer — readers and writers both use it.
- `agmem prompt` (adapters/cli/prompt.go) is the user-facing usage contract — keep it in sync with any surface change.
- English everywhere, conventional commits, transparent naming.

## Workflow

```sh
make build   # bin/agmem
make test    # all tests — no docker, sqlite is in-memory
make lint    # gofmt + go vet + golangci-lint — required before commit
```
