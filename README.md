# go-agent-memory

Harness-agnostic memory for AI agents. Research results, preferences, decisions, and locations live in **one SQLite file**, findable by full-text search, keywords, and dates — ranked by how useful past agents found them — so a fresh agent discovers what earlier agents learned, regardless of which model or harness either ran on.

One binary, `recall`: three long-lived faces behind `recall run`, one-shot commands for everything else.

- **MCP server** — `recall run` (stdio, the default face): `store_memory`, `search_memory`, `get_memory`, `rate_memory`, `delete_memory`, `pack` in any MCP-capable harness. Tools answer in plain text and say only what the next decision needs — the surface is tuned for what it costs a model to carry.
- **CLI** — `recall memory ...`, for agents that shell out: JSON output when piped, exit code 0/1 with machine-readable errors, never a prompt.
- **TUI** — `recall run -t`, for humans: browse and live-search, read, rate, prune, correct via a pre-filled supersede editor, review downvoted candidates (`R`), and manage the remote endpoint — all against local or hosted memory alike.

## Install

Needs Go 1.25+ (`go version`); pure Go, no cgo, macOS and Linux.

```sh
go install github.com/KucherenkoIvan/go-agent-memory/cmd/recall@latest
# or from a clone — same destination, and backs up the db before upgrades:
make install
```

Both drop `recall` into `$(go env GOBIN)`, or `$(go env GOPATH)/bin` when that is empty — usually `~/go/bin`. Put that directory on `PATH`, then check with `recall --version`. Memory lives in `~/.local/share/recall/memory.db`, created on first use.

Wire it into your harness as an MCP server, with the **absolute path** — GUI apps often don't inherit your shell's `PATH`. `realpath "$(go env GOPATH)/bin/recall"` prints it (use `GOBIN` if you set one). Register it once per harness, globally, so every project shares one memory:

**Claude Code**

```sh
claude mcp add --scope user recall -- "$(go env GOPATH)/bin/recall" run
claude mcp list    # recall: … ✔ Connected
```

**Cursor**: the IDE and the `agent` CLI both read `~/.cursor/mcp.json` (`.cursor/mcp.json` in a project scopes it to that project). Merge in:

```json
{"mcpServers": {"recall": {"command": "/home/you/go/bin/recall", "args": ["run"]}}}
```

Check with `agent mcp list` (`recall: ready`). Headless `agent -p` runs need `--approve-mcps` to load it.

**Codex**: the CLI, IDE extension, and desktop app share `~/.codex/config.toml`:

```sh
codex mcp add recall -- "$(go env GOPATH)/bin/recall" run
```

or add the table by hand:

```toml
[mcp_servers.recall]
command = "/home/you/go/bin/recall"
args = ["run"]
```

**Anything else MCP-capable**: the same `{"command": "<absolute path>", "args": ["run"]}`.

The MCP tools carry their own usage policy, so nothing else is needed. Agents without MCP shell out instead: paste `recall prompt` into their global instructions (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, or Cursor's User Rules).

## Quick start

```sh
# teach shell-out agents the contract — paste the output into AGENTS.md/CLAUDE.md:
recall prompt

# the loop agents run — keywords are positional and OR-match (more matches rank higher):
recall memory pack project:myapp lint           # session bootstrap: ranked context block
recall memory search go lint --text "golangci"  # ranked summaries; --text layers FTS on top
recall memory get <id>...                       # full content (counts as usage)
recall memory rate <id>... up                   # feedback drives future ranking
recall memory store --summary "one line" -k go -k lint --ttl 2160 "the finding..."
recall memory store --supersedes <old-id> ...   # corrections replace, never edit
recall memory delete <id>                       # obsolete beyond correcting: gone for good
```

## Shared memory (hosted mode)

The same binary hosts and consumes shared memory over gRPC. An API key both authenticates a caller and selects its **space**; spaces are hard-isolated, one SQLite file each, and a space file is byte-identical to a local `memory.db`.

```sh
# on the server (plaintext gRPC — private network or TLS proxy in front):
recall run -s --addr :7846                             # data in ~/.local/share/recall/server
recall server keys create --space team-x --name ivan   # prints the key exactly once
recall server keys revoke <id>                         # one teammate out, others unaffected
recall server spaces export team-x ./team-x.db         # live-safe snapshot = a local memory.db

# on any client — every face (CLI, MCP, TUI) follows automatically:
recall remote set memory.example:7846 rcl_...   # verified before saving
recall remote status                            # server + auth probes, key masked
recall remote unset                             # back to the local file
```

Stored memories' `source` becomes the key's name server-side, so shared writes attribute themselves. Migrate-to-local is `server spaces export` + `RECALL_DB=<file>`; migrate-to-hosted is copying a local `memory.db` into `spaces/<name>.db`.

## How it behaves

- **The memory is global by default**: `~/.local/share/recall/memory.db`, overridable with `RECALL_DB`. Cross-session knowledge is the point.
- **Concurrent by design**: a harness-connected `recall run` MCP server and CLI calls (yours, subagents', other harnesses') safely share the file — WAL + immediate write transactions underneath. Same machine only; never put the file on NFS.
- **Agents never edit** — corrections supersede (the old memory keeps existing, drops out of default search, `--all` shows everything); what is obsolete beyond correcting is deleted outright, by agent or human.
- **Everything expires**: `--ttl <hours>` is required at store time — the writer decides how long the fact stays useful (max 10 years). Expired memories leave default search like superseded ones; `--all` still finds them.
- **Keywords are the only axis** — lowercase, free-form, unlimited. `kind:decision`, `project:myapp`, `task:lint` are conventions, not schema: `kind` was a required column until it proved to be a keyword wearing one.
- **Ranking** = text relevance (FTS5) ⊕ explicit ratings (dominant) ⊕ recency ⊕ access counts. Rate what you use; future agents benefit.
- **Summaries are required**: search returns summaries, `get` returns bodies — retrieval stays cheap to scan.

## Architecture

Built on [go-kernel](https://github.com/KucherenkoIvan/go-kernel) (DDD + hexagonal, from the [tinycore template](https://github.com/KucherenkoIvan/go-tinycore-template)): the `Memory` aggregate owns the invariants, MCP/CLI are thin transport adapters over one `Service` facade — which is exactly what makes remote mode a drop-in: `recall remote set` swaps the local implementation for a gRPC client and no face notices. The gRPC contract lives in the kernel's `contracts/` tree. One documented deviation: FTS5 is maintained by SQL triggers, because the file is shared by multiple processes and in-process events can't index foreign writes. See [DESIGN.md](DESIGN.md) for the full design and phases.

## License

[0BSD](LICENSE).
