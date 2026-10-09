---
name: leadyard-verdict
description: leadyard scope audit and verdict. Audit the final diff for unrequested additions, compute the evidence level, and present the verdict package to the human. Use when a leadyard task is "verifying" and the review is done, or when the human asks if the task is ready.
license: MIT
---

# Scope audit and verdict

1. `leadyard scope`: lists changed files the plan does not mention, protected paths, zones
   touched, dependency manifests. Add your own check of the final diff for hardcoded values and
   departures from recorded decisions. Ask the human "remove or keep (with reason)" per item.
2. If a zone raised the class above the confirmed one, tell the human; they confirm the new class.
3. `leadyard level` is the verdict package. Show it to the human as is, plus one line each:
   what was not checked and why. Do not restate it as "everything works".
4. `leadyard transition ask_human --by agent --reason "verdict"` and wait.
5. Record only what the human says:
   - accept: `leadyard transition approve_verdict --by human:<name> --reason "<their words>"`
   - accept below the requirement: `leadyard transition accept_with_risk --by human:<name> --reason "<their words>"`
   - return: `leadyard transition request_changes --by human:<name> --reason "<their words>"`
