---
name: leadyard-pr
description: leadyard merge package. Build the draft pull request description from the task's evidence. Use when a leadyard task is "ready_to_merge" or the human asks for a PR description.
license: MIT
---

# PR body

1. Optional: add `## Release notes` to task.md with the deploy order and notes for QA. They are
   printed as "written by the agent".
2. `leadyard pr-body > /tmp/leadyard-pr-<task>.md`. Verified lines come from evidence records.
3. Delivery `pr`: `gh pr create --draft --body-file /tmp/leadyard-pr-<task>.md --title "..."`.
   Other modes: give the human the file; they open the PR.
4. Never merge, approve, or mark the draft ready. When the human reports the merge:
   `leadyard transition merged --by human:<name>`.
