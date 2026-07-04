# go-agent-memory — Design

A harness-agnostic memory system for AI agents: research results, preferences, decisions, and locations stored in SQLite, findable by keyword, full-text search, and date — ranked by how useful past agents found them — so a fresh agent discovers what earlier agents learned, regardless of which model or harness either ran on.

Local-first: the memory **is one `.db` file**, no daemon required. Optionally hosted: the same core behind a gRPC server with API keys, accessed by the same CLI/TUI/MCP faces. Working name: repo `go-agent-memory`, binary **`agmem`**.

## Faces

- **MCP server** (`agmem mcp`, stdio) — the flagship adapter: one config line in any MCP-capable harness.
- **CLI** (`agmem store|search|get|rate|recall ...`) — for harnesses that shell out: one-shot commands, stable JSON output, exit codes, no prompts.
- **TUI** (`agmem tui`) — the human face: browse, search, read, correct, prune.
- **Server** (`agmem serve`, phase 3) — gRPC hosting for shared/team memory; see Remote mode.

All faces sit on a small **application facade** with two implementations: *local* (in-process use-cases over the file) and *remote* (gRPC client) — so `--remote host:port --api-key ...` makes the same CLI/TUI/MCP talk to a hosted memory. This facade exists from day one even though remote ships later.

## Domain

**`Memory`** — the only aggregate:

| Field | Notes |
|---|---|
| content | markdown, non-empty |
| **summary** | one line; what search results show — agents scan summaries cheaply, then `get` the full body |
| kind | `fact` / `preference` / `research` / `decision` / `location` / `reference` |
| keywords | normalized, unlimited; hierarchy conventions live here (`project:go-kernel`, `task:lint`) — **there is no tree** (see below) |
| source | free-form writer identity (harness/model/session); server mode derives it from the API key |
| **ttl (hours, optional)** | time-boxed facts ("deploy freeze until Friday"). Expired memories drop out of default search like superseded ones; physical pruning is a later policy |
| rating signals | up/down votes (explicit feedback via `rate`) + access count / last-accessed (implicit) |
| supersede link | agents never edit — a correction **supersedes** (new memory pointing at the old, which leaves default search). Humans may hard-edit/delete via TUI |

**No `Node` tree.** Cut on cold-start grounds: a hierarchy nobody has populated is a hierarchy nobody will query, and everything a `project/task/topic` node would say is already a keyword. Conventional keyword prefixes give hierarchy-like scoping for free and require no ceremony from agents. If real grouping needs emerge, they materialize as *views over keywords*, not schema.

Events (`MemoryStored/Superseded/Rated`) remain for in-process reactions (TUI refresh, stats, future policies).

## Ranking

Search order = weighted blend, computed in SQL, tunable constants in one place:

```
score = bm25(FTS match) ⊕ rating (upvotes − downvotes, damped) ⊕ recency ⊕ access count (log-damped) ⊕ keyword matches (recall: keywords OR-filter, each match boosts)
```

Explicit feedback dominates implicit: a memory that agents *said* helped outranks one that merely got fetched. Downvoted-into-the-ground memories surface in a TUI "review candidates" view rather than being auto-deleted. No ML, no embeddings in MVP — sqlite-vec behind the same reader port later if FTS proves insufficient.

## Agent flows (the product, really)

**Discover:** `search` with task keywords → ranked list of *summaries* + snippets + ids (cheap to scan) → `get` the few that matter → after using one, `rate` it. For cold starts: `search` with no query returns the recent timeline.

**Recall pack:** `recall(keywords, budget)` — the one-call version: assembles the top-ranked memories into a single markdown block trimmed to a size budget, ready to drop into context. This is the tool agents will actually reach for at session start.

**Store:** finish work → `store` with content, one-line summary, keywords, kind, optional ttl. **Search-before-store is the convention** (avoid near-duplicates); updating existing knowledge = `store --supersedes <id>`.

**Teach the agents:** `agmem prompt` prints a recommended instructions block (when to search, how to store, keyword conventions, rating etiquette) for pasting into a harness's AGENTS.md/CLAUDE.md — the system documents its own usage contract.

## Storage decisions

- **FTS5 via SQL triggers, not a projector** — the file is multi-process (MCP server + CLI one-shots), and in-process events can't index another process's writes. Consistency mechanics live in the storage layer when storage is shared.
- **Multi-process local SQLite is the deployment model** — a harness-connected MCP server and CLI one-shots (any harness, any subagent) run concurrently against the global file. Safe because the kernel's sqlite client sets WAL + busy_timeout + `BEGIN IMMEDIATE` write transactions (kernel ≥ v0.11.1; immediate transactions eliminate the deferred-upgrade race, which silently *loses updates*, not just errors). Same machine only — never NFS.
  *History: v2 of this design used a flock to enforce single-process instead; reversed once `agmem mcp` living in a harness config made the lock hold for entire sessions, blocking every CLI call meanwhile.*
- Implicit access-count updates are fire-and-forget single-statement writes (no transactions on the read path).

## Remote mode (phase 3 — shipped)

`agmem serve` hosts shared memory over gRPC via the kernel (`grpckit` + `app.Run` + gRPC health + reflection). The contract lives in **go-kernel's `contracts/`** tree (`grpc/agmem/v1`); agmem imports the generated package.

**Tenancy: named spaces, N API keys per space.** A key (hashed at rest, `agm_`-prefixed, checked by a `grpckit.WithUnaryInterceptor`) both authenticates and selects its space; the key's name becomes `source`, so shared memory attributes writers automatically. Revoking one teammate's key never rotates the others.

**One SQLite file per space** (`<server-dir>/spaces/<name>.db`, names locked to `[a-z0-9-]+`): isolation by construction — no space column, no WHERE-clause to forget — and every space file is byte-identical to a local `memory.db`. Migrate-to-local is `agmem spaces export` (live-safe `VACUUM INTO`) + `AGMEM_DB=<file>`; migrate-to-hosted is copying a local file in. A lazy `SpaceRegistry` opens spaces on first use; `keys.db` (own migration set) is the control plane.

**Client side:** `agmem remote set <addr> <key>` (verified before saving; config at `~/.config/agmem/remote.json`, 0600; `AGMEM_REMOTE_ADDR`/`AGMEM_API_KEY` override per-field) swaps the local Service for the gRPC client inside `connect()` — every face (CLI, MCP, TUI) follows automatically, and typed domain errors survive the wire. Transport is plaintext: private network or TLS-terminating proxy.

## Surfaces (sketch)

MCP tools / CLI subcommands (1:1): `store_memory(content, summary, kind, keywords[], ttl_hours?, supersedes?, source)` · `search_memory(query?, keywords[]?, kind?, since?/until?, limit, all?)` · `get_memory(id)` · `rate_memory(id, up|down)` · `recall(keywords[], budget)` · plus CLI-only `agmem prompt`, `agmem tui`, `agmem serve`.

## MVP → later

**MVP (local):** `Memory` aggregate with summary/ttl/rating fields · FTS5 ranked search with all filters · supersede · `store/search/get/rate/recall` over MCP + CLI · `agmem prompt`. **Proof of done:** an agent stores research via MCP; a different harness finds it via CLI, ranked sensibly.
**Phase 2:** TUI (browse, review-candidates view, human edits) · stats/timeline.
**Phase 3:** hosted mode (`serve`, API keys, remote facade for all faces).
**Later:** retention/dedup policies · `SummarizerGateway` consolidation (optional LLM, never required) · export/import · sqlite-vec.
**Non-goals:** vector DB ambitions, harness-specific integrations beyond MCP + CLI, auto-deletion without human review.

## Open questions

1. **summary: required or optional?** Recommendation: **required** — it's cheap for the writer and it's what makes search results scannable; optional fields written by lazy agents stay empty.
2. Rating mechanics: are implicit access counts wanted at all, or explicit votes only? (Proposal: both, explicit-dominant.)
3. `recall` budget unit: characters (simple, model-agnostic) or approximate tokens? Proposal: characters.
4. Default DB location: global `~/.local/share/agmem/memory.db` with `--db`/env override (proposal), or per-project files?
