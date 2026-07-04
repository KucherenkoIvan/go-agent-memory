# Agent instructions

A single-binary service on [go-kernel](https://github.com/KucherenkoIvan/go-kernel). The kernel's [architecture docs](https://github.com/KucherenkoIvan/go-kernel/blob/master/docs/architecture/1-service-structure.md) and package guides are the source of truth — read them before restructuring anything.

Everything named `changeme` is placeholder scaffolding: rename it into the first real feature (or copy it for a new one and delete it), including the migration and tests. Do not build on top of the placeholder names.

## Hard rules

1. **Layer discipline** (lint-enforced where possible):
   - `domain/` imports only the stdlib and `go-kernel/ddd`. No infra, no HTTP, no sql.
   - `application/` imports domain and ports — never `adapters/` or `shared/infra/`.
   - Features never import other features; cross-feature needs go through `internal/shared/ports/`.
   - Transport frameworks stay inside their adapters (`gin.Context` never leaves `adapters/rest/`; proto types never leave `adapters/grpc/`; tea types never leave `adapters/tui/`); use-cases take `context.Context` + typed arguments.
   - Presentation binaries are composition roots: `cmd/app` (server) and `cmd/tui` (terminal) share features via `Feature.UseCases`; new presentation = new `cmd/` + adapter, never a fork of the feature.
   - Queries go through Reader ports and read-models — never through repositories.
2. **Invariants live in aggregate methods** — never duplicated in use-cases or handlers. Expected business failures are `ddd.DomainError` values; transports map them (`httpapi.WithErrorStatus` for non-400s).
3. **Schema changes are migrations** in `internal/shared/infra/storage/migrations/` — numbered, up-only, never edited after commit.
4. **There is no CI — you are the CI.** `make lint` and `make test` must pass before every commit. Tests use real components (`:memory:` sqlite, the channel publisher) — prefer them over mocks; port fakes are hand-written maps, not mock frameworks.
5. **New feature checklist**: copy the `changeme/` layout (domain → ports → use-cases → adapters → `feature.go`), add its migration, define its gRPC contract (own proto — the changeme contract in the kernel is template scaffolding), wire routes/registration and domain-error mappings (`WithErrorStatus`/`WithErrorCode`) in `cmd/app/main.go`, register health checks for any new infra, add tests at domain and transport altitude.

## Conventions

- English everywhere. Conventional commits. Transparent naming — call things what they are.
- Config via the typed struct in `main.go` only; no `os.Getenv` outside it.
- Events are past-tense facts published inside the business transaction (commit-gated by the kernel); projectors only write read-models; reactive logic that triggers commands is a policy.
- Queries read through Reader ports and read-models (add them per the kernel's application-layer guide when read scenarios appear) — never through repositories.

## Workflow

```sh
make run    # start the service (HTTP_ADDR/DB_PATH from .env)
make test   # all tests — no docker needed, sqlite is in-memory
make lint   # gofmt + go vet + golangci-lint — required before commit
make build  # static binary into bin/
```
