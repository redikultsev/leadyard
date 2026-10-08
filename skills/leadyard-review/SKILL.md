---
name: leadyard-review
description: leadyard review stage. Get an independent review in a fresh session from a bundle the CLI builds, and record its verdict. Use when a leadyard task has status "verifying" and no fresh review is recorded.
license: MIT
---

# Review

The reviewer must not see your implementation conversation, and you must not write its
instructions: the CLI builds them.

1. `leadyard review input > /tmp/leadyard-review-<task>.md`.
2. Start a fresh subagent (or a new session) with read-only tools. Give it exactly that file
   as the prompt, nothing else. Do not summarize or add hints.
3. Save the subagent's full answer to a file and run
   `leadyard review record --file <answer-file>`. The CLI reads the last `leadyard-verdict`
   block, assigns finding ids, and keeps earlier findings open until the reviewer reports them
   fixed. A missing block is an error: run the review again.
4. Show the human the open findings. They decide which to fix. A finding they dismiss:
   `leadyard decide finding_dismissed --ref <id> --by human:<name> --reason "<their words>"`.
5. Fix the chosen findings (implement skill), then review again. The CLI stops automatic
   rounds at the project's limit; after that the human decides.
