# go-agent-memory — Design

A harness-agnostic memory system for AI agents: research results, preferences, decisions, and locations stored in a single SQLite file, findable by keyword, date, and a project/task tree — so a fresh agent can discover what past agents learned, regardless of which model or harness either of them ran on.

The memory **is one `.db` file**. No daemon required, no vendor lock, no dependence on any harness's built-in memory. Working name: repo `go-agent-memory`, binary **`agmem`**.

## Faces

Same split as everything in this family — machine faces and a human face over one core:

- **MCP server** (`agmem mcp`, stdio) — the flagship adapter: any MCP-capable harness gets `store_memory` / `search_memory` / tree tools by adding one line to its server config.
- **CLI** (`agmem store|search|get|tree ...`) — for harnesses/agents that prefer shelling out: one-shot commands, stable JSON output, exit codes, no prompts. Same agent contract rules as the explorer design.
- **TUI** (`agmem tui`) — for the human: browse the tree, search, read, prune, correct what agents wrote.

One binary, subcommands select the face — an MCP config points at `agmem mcp` and nothing else needs installing.

## Domain

| Concept | Kind | Notes |
|---|---|---|
| `Memory` | aggregate | content (markdown), kind (`fact` / `preference` / `research` / `decision` / `location` / `reference`), keywords, source (harness/model/session that wrote it), optional node attachment, timestamps. Invariants: non-empty content, valid kind, keyword normalization |
| supersede | aggregate behavior | memories are never edited by agents — a correction **supersedes** (new memory linking the old; old drops out of default search). Humans may hard-edit/delete via TUI |
| `Node` | aggregate | the project/task tree: name, kind (`project` / `task` / `topic`), parent. Invariants: parent exists, no cycles, no self-parenting |
| `MemoryStored/Superseded`, `NodeCreated/...` | events | feed stats and future policies |
| search, timeline, tree, stats | read-models | keyword + full-text + date-range + node-scope + kind filters, ranked |

Feature split: **`memories`** and **`tree`**, with `memories → tree` through a shared port (attachment validation) — or one feature if the boundary feels forced during implementation; decided by code, not by this doc.

## The two design decisions that matter

**1. Full-text search: FTS5 via SQL triggers, not a projector.** modernc's SQLite includes FTS5. The architecture-pure move would be an index-projector on `MemoryStored` — but this database is deliberately **multi-process** (a long-running MCP server *and* one-shot CLI invocations touching the same file), and in-process events can't index another process's writes. DB-level triggers keep the FTS index atomic with every writer. This is a conscious, documented deviation: consistency mechanics belong to the storage layer when the storage is shared; events remain for in-process reactions (TUI refresh, stats, future policies).

**2. Multi-process SQLite is allowed here — deliberately relaxing the template rule.** The "one process per file" rule guards *service replicas*; local same-machine multi-process access is exactly what WAL is designed for. Rules that make it safe: WAL + busy_timeout (kernel defaults), short transactions, all writers on the same machine (the file never lives on NFS). Documented in the README as a first-class property, since it's the whole deployment model.

## Self-management (explicitly out of MVP)

Retention/decay, near-duplicate detection, LLM summarization/consolidation — all designed as **policies behind ports** (`SummarizerGateway` that an adapter may implement with any model, or not at all). The system must be fully useful with zero LLM calls of its own; that's part of harness-agnosticism. Ships as increments after the core proves itself.

## Surfaces (sketch)

MCP tools: `store_memory(content, kind, keywords[], node?, source)` · `search_memory(query?, keywords[]?, kind?, node?, since?/until?, limit)` · `get_memory(id)` · `supersede_memory(id, content, ...)` · `create_node(name, kind, parent?)` · `get_tree(root?)` · `attach_memory(id, node)`.
CLI mirrors the tools 1:1 (`agmem store -k api -k auth --kind research "..."`); `--output json` default on non-TTY.
SDK: the official `modelcontextprotocol/go-sdk` (stdio transport) — `mark3labs/mcp-go` is the fallback if the official one disappoints.

Search semantics: FTS5 ranked match over content+keywords, filters compose (AND), superseded excluded unless `--all`, results carry id/kind/keywords/date/node-path/snippet.

## MVP → later

**MVP:** `Memory` + `Node` aggregates · FTS5 search with all filters · supersede semantics · MCP server (tools above) · CLI · multi-process-safe storage. **Proof of done:** an agent session stores research via MCP, a *different* harness finds it via CLI.
**Later:** TUI browser · stats/timeline views · retention & dedup policies · `SummarizerGateway` consolidation · export/import (markdown dump) · embeddings via sqlite-vec behind the same reader port.
**Non-goals:** being a vector DB, multi-user/server deployment, harness-specific integrations beyond MCP + CLI.

## Open questions

1. Naming: `agmem` as the binary?
2. Where does the `.db` live by default — `~/.local/share/agmem/memory.db` (one global memory) with `--db`/env override per project, or per-project files by default? Global-with-override is my proposal (the point is cross-session knowledge).
3. Should `source` (who wrote it) be free-form text the caller provides, or a structured convention (`harness/model/session`)? Free-form string in MVP is my proposal.
4. Kinds list above good enough to start? (They mirror the categories you named: research results, preferences, locations — plus fact/decision/reference.)
