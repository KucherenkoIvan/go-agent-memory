package cli

// agentPrompt is what `recall prompt` prints — the usage contract to paste
// into a harness's AGENTS.md/CLAUDE.md. The system documents itself.
const agentPrompt = `## Persistent memory (recall)

A shared memory survives across sessions, models, and harnesses. Use it via the ` + "`recall`" + ` CLI (or the equivalent MCP tools if configured). Use it on your own initiative — nobody will prompt you: recall at task start, rate what you used, store what you learned before finishing.

**At session start** — recall what past agents learned about your task:

    recall pack -k <topic> -k <topic>         # ranked memories as one block
    recall search -q "<free text>" -k <topic> # or browse summaries first

Pack keywords OR-match: throw in every candidate topic (project, task, technology) — memories matching more of them rank higher. Search keywords AND-match: use them to narrow.

If recall returns nothing relevant, fall back to your harness's builtin memory (if you have one) — and store anything useful you find there into recall so it is shared next time.

**When you rely on a memory** — rate it afterwards; ratings drive ranking for future agents:

    recall rate <id> up      # it was correct and helped
    recall rate <id> down    # it was wrong or misleading

**When you learn something worth keeping** (a research result, a decision and its why, a preference, a location of something), store it — but search first to avoid duplicates:

    recall store --summary "<one line>" --kind <fact|preference|research|decision|location|reference> \
      -k <topic> -k <topic> [--ttl <hours>] [--supersedes <id>] "<content>"

Rules:
- keep memories small and atomic: one fact per memory, sharp keywords. Split a big finding into several notes — search and ranking reassemble them better than one document.
- summary is required: one line, it is what future agents see in search results.
- keywords: lowercase topics; use prefixes for scoping, e.g. project:myapp, task:lint.
- never store secrets or credentials.
- to correct an existing memory, store the correction with --supersedes <old-id> — do not duplicate.
- use --ttl for time-boxed facts ("freeze until Friday").
- output is JSON when piped; exit code 0 = success, 1 = error (JSON on stderr).`
