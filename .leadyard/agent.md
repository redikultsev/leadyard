# leadyard (managed by leadyard init; edits are overwritten)

This repository uses leadyard. The `leadyard` CLI owns task state and evidence.

- Start every task with the `leadyard` skill; after a context reset run `leadyard resume`.
- "Done" is the level printed by `leadyard level`, never your own judgment.
- Do not write .leadyard/config.yaml, .leadyard/zones.yaml or any evidence.jsonl; the gate blocks it.
- Follow the git delivery mode printed by `leadyard resume`. In mode none you run no git writes.
- Do only what the request and the plan ask: no extra flags, env vars, files or dependencies.
