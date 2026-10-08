---
name: leadyard-implement
description: leadyard implement stage. Change code following the plan, test first, and record evidence by running the project's checks through the leadyard CLI. Use when a leadyard task has status "in_progress".
license: MIT
---

# Implement

1. Classes 1–2 have no plan stage: first write a short `## Plan` (files you intend to change, in
   backticks, and an out-of-scope line). The scope audit compares the diff against it.
2. Work test first. For a fix: write a test that fails, run it, then fix, then run it again.
   Do not edit a test to make it pass; if a pre-existing test must change, say why in Notes —
   the human will be asked to accept the edit.
3. Do only what the request and plan ask. No extra flags, env vars, files, dependencies or
   hardcoded values. If something more seems needed, ask.
4. Tick a checkbox right after its step. A checkbox shows progress; it is not acceptance.
5. Record evidence only through the CLI: `leadyard run <check-id> [--criteria C1,C2]`.
   `leadyard config` lists the checks. Pass `--criteria` for the acceptance criteria a check
   actually exercises.
6. Git follows the delivery mode (`leadyard resume`). In mode `none` you never commit.
7. Report caveats plainly ("done, except X" means not done). Then
   `leadyard transition verify --by agent`.
