package cli

// agentPrompt is what `recall prompt` prints — the usage contract to paste
// into a harness's AGENTS.md/CLAUDE.md. The system documents itself.
const agentPrompt = `## Persistent memory (recall)

A shared memory survives across sessions, models, and harnesses. Use it via the ` + "`recall`" + ` CLI (or the equivalent MCP tools if configured). You may be asked to use it explicitly, but you do not need to be — rate what you used and store what you learned on your own initiative.

**When past work plausibly touched your task** (familiar project, recent changes led here, the issue sounds recurrent) — recall what earlier agents learned. Skip this on clearly fresh ground; a reflexive pack pulls irrelevant context in:

    recall memory pack <topic> <topic>...        # ranked memories as one block
    recall memory search <topic>... [--text "…"] # or browse summaries first

Keywords OR-match in both: throw in every candidate topic (project, task, technology) — memories matching more of them rank higher. --text layers a full-text query over the keyword results; --and (search only) requires every keyword when you need to narrow hard.

If recall returns nothing relevant, fall back to your harness's builtin memory (if you have one) — and store anything useful you find there into recall so it is shared next time.

**When you rely on a memory** — rate it afterwards; ratings drive ranking for future agents:

    recall memory rate <id> up      # it was correct and helped
    recall memory rate <id> down    # it was wrong or misleading

**When you learn something worth keeping** (a research result, a decision and its why, a preference, a location of something), store it — but search first to avoid duplicates:

    recall memory store --summary "<one line>" --kind <fact|preference|research|decision|location|reference> \
      -k <topic> -k <topic> [--ttl <hours>] [--supersedes <id>] "<content>"

Rules:
- prefer small, focused memories: each should cover one self-contained finding or topic, with sharp keywords. Split a sprawling write-up into several notes — search and ranking reassemble them better than one document.
- summary is required: one line, it is what future agents see in search results.
- keywords: lowercase topics; use prefixes for scoping, e.g. project:myapp, task:lint.
- never store secrets or credentials.
- to correct an existing memory, store the correction with --supersedes <old-id> — do not duplicate.
- use --ttl for time-boxed facts ("freeze until Friday").
- output is JSON when piped; exit code 0 = success, 1 = error (JSON on stderr).
- fetch full content with: recall memory get <id>`
