package cli

// agentPrompt is what `agmem prompt` prints — the usage contract to paste
// into a harness's AGENTS.md/CLAUDE.md. The system documents itself.
const agentPrompt = `## Persistent memory (agmem)

A shared memory survives across sessions, models, and harnesses. Use it via the ` + "`agmem`" + ` CLI (or the equivalent MCP tools if configured).

**At session start** — recall what past agents learned about your task:

    agmem recall -k <topic> -k <topic>       # ranked memories as one block
    agmem search -q "<free text>" -k <topic> # or browse summaries first

**When you rely on a memory** — rate it afterwards; ratings drive ranking for future agents:

    agmem rate <id> up      # it was correct and helped
    agmem rate <id> down    # it was wrong or misleading

**When you learn something worth keeping** (a research result, a decision and its why, a preference, a location of something), store it — but search first to avoid duplicates:

    agmem store --summary "<one line>" --kind <fact|preference|research|decision|location|reference> \
      -k <topic> -k <topic> [--ttl <hours>] [--supersedes <id>] "<content>"

Rules:
- summary is required: one line, it is what future agents see in search results.
- keywords: lowercase topics; use prefixes for scoping, e.g. project:myapp, task:lint.
- never store secrets or credentials.
- to correct an existing memory, store the correction with --supersedes <old-id> — do not duplicate.
- use --ttl for time-boxed facts ("freeze until Friday").
- output is JSON when piped; exit code 0 = success, 1 = error (JSON on stderr).`
