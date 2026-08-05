package cli

// agentPrompt is what `recall prompt` prints — the usage contract to paste
// into a harness's AGENTS.md/CLAUDE.md. The system documents itself.
//
// Keep it short. Harnesses running the MCP face already receive this policy
// in the server's instructions block; every line here is a line paid for in
// every session of every agent that pastes it.
const agentPrompt = `## Persistent memory (recall)

A shared memory survives across sessions, models, and harnesses. Use it on your own initiative — you do not need to be asked. (Configured as an MCP server? The tools carry this policy already; keep this block short or drop it.)

**Recall what past agents learned** when the task plausibly touches earlier work — a familiar project, recent changes that led here, a recurrent-sounding issue. Skip it on fresh ground:

    recall memory pack <topic> <topic>...        # ranked memories as one block
    recall memory search <topic>... [--text "…"] # or browse summaries first

Keywords OR-match in both: throw in every candidate topic (project, task, technology) — memories matching more of them rank higher. --text layers a full-text query on top; --and requires every keyword; --offset pages; --order swaps relevance for created_asc|created_desc|rating_asc|rating_desc|reads_asc|reads_desc. Search again mid-session when the work shifts — the opening pack does not cover topics that came up later.

**Rate what you relied on** — ratings drive ranking for everyone:

    recall memory rate <id>... up      # correct and helpful
    recall memory rate <id>... down    # wrong or misleading

**Store the moment you learn something durable** — a root cause, an environment or tooling gotcha, a decision and its why, a stated preference, a location. Not at end-of-session: compaction loses findings. Search first to avoid duplicates:

    recall memory store --summary "<one line>" -k <topic> -k <topic> --ttl <hours> \
      [--supersedes <id>] "<content>"

**Prune what should not exist** — the code or process it describes is gone, the decision was reverted, it was never true:

    recall memory delete <id>          # permanent, unconfirmed

Rules:
- if your harness has builtin memory, recall is still the primary store: it is shared, the builtin is not.
- keep memories small and single-topic with sharp keywords — search reassembles them better than one long note.
- summary is required: one line, it is what future agents see in search results.
- keywords are lowercase and free-form; kind:decision, project:myapp, task:lint are conventions, not a vocabulary.
- --ttl is required: how long the fact stays true (720 = 30 days, 8760 = a year, max 10 years). Expired memories leave default search; --all brings them back.
- supersede when knowledge changed (--supersedes <old-id>), delete when the subject is gone. Never duplicate.
- never store secrets or credentials.
- output is JSON when piped; exit 0 = success, 1 = error (JSON on stderr).
- full content: recall memory get <id>...`
