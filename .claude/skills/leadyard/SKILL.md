---
name: leadyard
description: Run a task through the leadyard loop (intake, clarify, plan, implement, review, scope audit, verdict, PR body) with evidence levels computed by the leadyard CLI. Use when the user gives a task, ticket or bug to work on in a repository that has a .leadyard/ directory, or asks "is it ready", "what's the status", "continue the task".
license: MIT
---

# leadyard loop

The `leadyard` CLI owns the task state. You never write `evidence.jsonl`, `events.jsonl`,
`.leadyard/config.yaml` or `.leadyard/zones.yaml` yourself; the gate blocks it.

1. Find the task: `leadyard task show` (current task) or create one:
   `leadyard task new --id <TRACKER-KEY> --title "..." --kind feature|fix|chore --request-file <file>`.
2. Read `leadyard resume`. It prints the status and the next step. Follow it.
3. Stages, in order. Each has its own skill:
   - intake → `leadyard-intake`
   - clarify → `leadyard-clarify` (classes 2–3)
   - plan → `leadyard-plan` (class 3; classes 1–2 write a short plan in implement)
   - implement → `leadyard-implement`
   - review → `leadyard-review` (classes 2–3)
   - scope audit and verdict → `leadyard-verdict`
   - PR body → `leadyard-pr`
4. Move the status only with `leadyard transition <event> --by agent`. Human events
   (`approve_verdict`, `accept_with_risk`, `request_changes`, `merged`, ...) are recorded with
   `--by human:<name>` only after the human said so in this chat, quoting their words in `--reason`.
5. "Is it ready?" is answered only with `leadyard level`, never from memory.
6. Keep `## Notes` in task.md current: hypotheses, running jobs, what you found. It survives
   context compaction (`leadyard resume` prints it).

Git: follow the delivery mode in `leadyard resume`. In mode `none` you never run git write
commands; the human commits.
