# go-tinycore-template

A template for single-binary Go services on [go-kernel](https://github.com/KucherenkoIvan/go-kernel): DDD + hexagonal architecture in one static executable — embedded SQLite, commit-gated in-process events, graceful runtime, health endpoints. No containers, no external dependencies at runtime; the database is a file next to the binary.

Everything named **`changeme`** is deliberate scaffolding, not a domain: a nameless aggregate with a placeholder field and invariant, CRUD commands + CRUD events, `get`/`list` queries through a reader, a sqlite repository — and both transports wired end to end: a REST CRUD API (Gin) and a gRPC service (contract in the kernel's `contracts/proto/grpc/changeme/v1`). It exists so your first real feature is a rename-and-fill, not a blank page — and so the wiring (transactions, event publishing, error mapping on both transports, migrations, composition root) is already correct and tested.

## Using the template

1. **Create your repo from it** (GitHub → "Use this template", or `gh repo create myapp --template KucherenkoIvan/go-tinycore-template`).
2. **Rename the module**: replace `github.com/KucherenkoIvan/go-tinycore-template` in `go.mod` and all imports, and update the depguard paths in `.golangci.yml`.
3. **go-kernel is a private module** — one-time setup:

	```sh
	go env -w GOPRIVATE=github.com/KucherenkoIvan/*
	git config --global url."git@github.com:KucherenkoIvan/".insteadOf "https://github.com/KucherenkoIvan/"
	```

4. `make test && make run` — then try it:

	```sh
	curl localhost:8080/healthz
	curl -X POST localhost:8080/api/changeme -d '{"name": "first"}'
	curl localhost:8080/api/changeme               # list
	curl localhost:8080/api/changeme/<id>          # single by id
	curl -X PUT localhost:8080/api/changeme/<id> -d '{"name": "second"}'
	curl -X DELETE localhost:8080/api/changeme/<id>
	# the same service also speaks gRPC on :9090 (ChangeMeService)
	```

5. Build your first feature by renaming `changeme` everywhere: aggregate, events, migration, adapters, tests — and its gRPC contract (define your own proto; the changeme one lives in the kernel only so the template compiles out of the box).

## Two binaries, one feature set

The template proves the multi-composition-root story: `cmd/app` is the server (HTTP + gRPC), **`cmd/tui`** is the same feature presented as a terminal application (Bubble Tea) — use-cases called in-process, sqlite underneath, no server at all. `make tui` to try it: `n` new, `e` edit, `d` delete, `↑/↓` move, `q` quit; committed domain events refresh the list live. One rule: one process per database file — don't point the TUI at a file the server is using.

## Layout

```
cmd/app/main.go                     # composition root: server (HTTP + gRPC + health + app.Run)
cmd/tui/main.go                     # composition root: terminal app (Bubble Tea owns the lifecycle)
internal/
  shared/infra/storage/             # embedded DB + migrations (schema source of truth)
  features/changeme/                # placeholder feature — rename or copy, then delete
    feature.go                      #   composition root: port -> adapter wiring
    domain/                         #   aggregate, CRUD events, read-model, domain errors
    application/ports/              #   repository + reader + event-producer contracts
    application/usecases/managechangeme/  # create/update/delete commands, get/list queries
    adapters/sqlite/                #   repository + reader
    adapters/rest/                  #   HTTP CRUD handlers (gin stays in here)
    adapters/grpc/                  #   gRPC controller (proto mapping only)
    adapters/tui/                   #   terminal UI (Bubble Tea stays in here)
```

The architecture is documented in the kernel: [service structure](https://github.com/KucherenkoIvan/go-kernel/blob/master/docs/architecture/1-service-structure.md) → domain → application → infrastructure, plus per-package guides. `AGENTS.md` carries the operating rules for coding agents.

## Growing out of tiny

Everything sits behind ports, so scaling up is adapter swaps, not rewrites: sqlite → postgres (write the pg twin of `adapters/sqlite/`), in-process events → transactional outbox + Kafka (same producer port), REST and/or gRPC transports when endpoints appear, Redis when the in-memory cache stops being enough. The kernel guides cover each move.

## License

[0BSD](LICENSE).
