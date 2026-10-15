---
id: 01M50GXCB1T4XHE0GR59NJ5SQ8
title: 'Windows: gate paths, gate tests, doctor, line endings'
kind: fix
status: ready_to_merge
class: 3
proposed_class: 3
attempt: 1
base: 13cc26d24b8cae3e71e0f3b0f447d301f236bf6c
created: 2026-10-15T19:34:04Z
---

## Request (verbatim)

Да, почини Windows-проблемы в клоне leadyard: пути в gate, тест, doctor и .gitattributes. Работай по циклу leadyard, ничего не пушь.

## Acceptance criteria

<!-- quoted from the ticket or the request: "- C1. ..." -->

- C1. On Windows the gate treats a Git Bash path (`/c/...`) as the Windows path it names: writes to
  `.leadyard/config.yaml`, `.leadyard/zones.yaml`, `evidence.jsonl`, `events.jsonl` and `.git/` are denied,
  protected paths ask. (derived from "пути в gate")
- C2. The gate's deny and ask checks do not depend on letter case: `.LEADYARD\CONFIG.YAML` is denied like
  `.leadyard/config.yaml`. (derived from "пути в gate")
- C3. `go test ./...` passes on Windows and on Linux; gate tests do not rely on Unix-style roots. (derived from "тест")
- C4. On Windows `leadyard doctor` runs its hook wrapper self-tests through Git's `sh` and reports no FAIL
  on a correct install. (derived from "doctor")
- C5. A clone made with `core.autocrlf=true` checks out LF files: `gofmt -l .` is empty and the skills
  front matter test passes. (derived from ".gitattributes")

## Plan

<!-- written by the plan step; checkboxes show progress, not acceptance -->

Branch `agent/01M50GXCB1T4XHE0GR59NJ5SQ8-windows` from `main` (decision "Branch and commits"). Test first: steps
1–2 and 5 are written before the code they test and must fail on Windows first.

- [x] 1. Gate tests: `internal/gate/gate_test.go` — `pol()` takes an OS-native root (`C:\repo` on Windows,
  `/repo` elsewhere) and `TestFileTools` builds its paths from it (finding 1, criterion C3). Add letter-case
  cases on every OS: Write `.LEADYARD/CONFIG.YAML`, `.Leadyard/Tasks/T-1/Evidence.JSONL`, `.GIT/config` →
  deny, `.GitHub/Workflows/ci.yml` → ask; shell `echo x > .LEADYARD/config.yaml` and
  `cp a .leadyard/TASKS/T-1/EVENTS.jsonl` → deny (C2; answer Q2 "регистр не различаем на всех ОС").
- [x] 2. Gate tests for Git Bash paths: a table test of the conversion on every OS (`/c/Users/x` →
  `C:\Users\x`, `/C` → `C:\`, `/usr/bin`, `/cygdrive/c/x`, `//server/share`, `c/x` unchanged); Windows-only
  cases: Write `/c/repo/.leadyard/config.yaml` → deny, `/c/repo/.github/workflows/ci.yml` → ask, shell
  `echo x > /c/repo/.leadyard/config.yaml` → deny (C1; finding 2).
- [x] 3. Gate code: `internal/gate/gate.go` — in `rel()` (gate.go:87), on Windows only, read a Git Bash drive
  path `/<letter>` or `/<letter>/...` as `<LETTER>:\...` before `filepath.IsAbs` (C1). Compare in lower case
  in `stateFile()`, the `.git/` check and protected-path matching (gate.go:102–126), messages keep the path as
  given (C2, answer Q2). `repo.Match` is not changed.
- [x] 4. Run `leadyard run fmt`, `leadyard run vet`, `leadyard run tests --criteria C1,C2,C3`.
- [x] 5. Doctor tests: new `internal/cli/doctor_test.go` — the hook input for a root with `\` is valid JSON
  with that root as `cwd` (finding 4); the Git sh lookup on a fake Git layout in a temp dir (`cmd/git.exe`
  + `usr/bin/sh.exe`, + `bin/sh.exe` only, nothing → not found); the minimal PATH for both OS families
  (C4).
- [x] 6. Doctor code: `internal/cli/doctor.go` — on Windows: no exec-bit check for the wrapper (doctor.go:80);
  the shell is `CLAUDE_CODE_GIT_BASH_PATH` when set, else Git's `usr\bin\sh.exe` or `bin\sh.exe` found from
  `git` on PATH, never a `bash` found on PATH (`System32\bash.exe` is WSL); PATH lists are Windows-style
  (`<leadyard dir>;<Git sh dir>`); HOME falls back to `os.UserHomeDir()`. On every OS the hook input is
  built with `encoding/json` (doctor.go:84, 97). No Git sh on Windows → FAIL with what to install or set
  (C4; finding 4; experiments in Notes).
- [x] 7. Line endings: `.gitattributes` — `* text=auto eol=lf` above the `# leadyard` lines (C5; finding 5).
- [x] 8. CI: `.github/workflows/ci.yml` — job `test` on `[ubuntu-latest, windows-latest]`, `fail-fast: false`,
  steps run under `bash` (protected path; answer Q4 "добавить windows-latest в CI").
- [x] 9. Decisions: `docs/decisions.md` — a dated section with new entries 66–69: Windows through Git for
  Windows (hooks and doctor under Git's sh; Git Bash drive paths read as Windows paths); case-insensitive
  gate checks on every OS, fail closed (answer Q2); known limits — other Windows path aliases (answer Q3) and
  `leadyard run` needing `sh` on PATH (answer Q5); LF checkout everywhere (AGENTS.md: "record new decisions
  in docs/decisions.md").
- [x] 10. Re-run the three checks with criteria (`tests` → C1–C4); rebuild `leadyard`; then, recorded in
  Notes as observations, not evidence: `leadyard doctor` from Git Bash and PowerShell (C4); the 15-input gate
  probe on the old binary (sha256 d106ad18…) and the new one (answer Q1); `go test ./...` on Linux in the
  official `golang:1.27.2` Docker image (C3 Linux half; an image download of a few hundred MB); a fresh
  clone of the branch with `core.autocrlf=true`: `git ls-files --eol`, `gofmt -l .`, skills test (C5).
- [x] 11. Commits on the task branch, Conventional Commits with `Assisted-by: claude-code/claude-opus-5-5`, no
  push: `fix(gate)`, `fix(doctor)`, `build`/`ci` and `docs`; the task folder after the verdict.
- Out of scope: `leadyard run` under PowerShell (Q5); trailing dots or spaces, `::$DATA`, 8.3 names,
  junctions (Q3); `leadyard init` line endings in target repositories (experiment: not needed); relative
  paths resolved against the root rather than the tool call's `cwd` (existing design); an end-to-end doctor
  test in `go test` (needs an installed binary); `docs/design.md` text; the Windows CI job is written but not
  run (nothing is pushed).

## Notes

<!-- working notes for resuming after compaction: hypotheses, running jobs, findings -->

Environment: Windows 11, Go 1.27.2, Git for Windows (`C:\Program Files\Git`, sh at `usr\bin\sh.exe`), global
`core.autocrlf=true`. This clone has `core.autocrlf=false` locally (re-checked out with LF). The Claude Code
session doing this task was started in another folder, so this repository's hooks do not guard it; the CLI
is used for every state change.

Prior work ("is it already done?"): no issues or PRs in redikultsev/leadyard; one remote branch `main`
(13cc26d); no commit mentions Windows, CRLF or MSYS; docs say nothing about Windows. My reading: not done.

Searched: `internal/gate/gate.go`, `internal/gate/git.go`, `internal/cli/doctor.go`, `internal/cli/hooks.go`,
`internal/cli/commands.go`, `internal/repo/fs.go`, gate and skills tests, `.gitattributes`, CI workflow.
Skipped: level, ledger, review, transition, task packages (not touched by the request).

Findings, each reproduced on this machine (2026-10-15):

1. Test `gate/TestFileTools` fails on Windows: `pol()` uses root `/repo`; `filepath.IsAbs("/repo/...")` is
   false on Windows, so `rel()` (gate.go:87) joins it under the root and 4 cases get `allow`.
2. Gate fails open for a Git Bash path: Write `/c/Users/User/projects/leadyard/.leadyard/config.yaml` →
   allow (same cause as 1). Windows paths with `\`, `C:/` and a lower-case drive are denied correctly.
3. Gate fails open on letter case: Write `C:\...\.LEADYARD\CONFIG.YAML` → allow; NTFS (and APFS by default)
   opens the same file. `stateFile()` and the `.git/` check compare case-sensitively (gate.go:102, 118).
4. `doctor` on Windows: FAIL "hook wrapper missing or not executable" — Go reports no exec bit for `.sh`
   (doctor.go:80); the wrapper self-tests are then skipped. They would also break: `exec.Command("/bin/sh")`
   (doctor.go:158), PATH built as `dir + ":/usr/bin:/bin"` (a drive letter has a colon), HOME read from
   `$HOME` (unset in PowerShell), and the hook input built by string concatenation, so a root with `\` is
   invalid JSON and the gate blocks every probe for the wrong reason (doctor.go:84, 97).
5. With global `core.autocrlf=true` a clone has CRLF files: `gofmt -l .` lists every file and
   `skills/TestFrontMatter` fails ("no front matter"); a binary built from that tree embeds CRLF skills.

Experiments (scratchpad, not in the repo):
- the hook wrapper with CRLF line endings still works under Git's sh (push to main → 2, git status → 0),
  so `init` needs no change for line endings in target repositories;
- Git's `sh.exe` started by a Windows process accepts a Windows-style PATH (`go\bin;Git\usr\bin`) and a
  POSIX one (`/usr/bin:/bin`), and finds `~/go/bin/leadyard` with HOME given as `C:\Users\User` or unset;
- `C:\Windows\System32\bash.exe` exists (the WSL launcher): a lookup of `bash` on PATH may find it, so the
  doctor must not take the first `bash` it sees.

Known limits outside this task (my reading): `leadyard run` uses `sh -c` and works on Windows only when `sh`
is on PATH (Git Bash, as Claude Code's Bash tool is); other Windows path aliases — trailing dots or spaces,
`::$DATA` streams, 8.3 short names, junctions — are not normalised by the gate.

Implementation (2026-10-15):
- Pre-existing test edited: `internal/gate/gate_test.go`. `pol()` and `TestFileTools` used the root `/repo`,
  which is not absolute on Windows, so the test could not pass there (finding 1). The root is now OS-native;
  every original case is kept with the same expectation; cases were only added. Needs the human's acceptance.
- Test first: the new gate cases failed on Windows before the fix (2 shell, 4 file-tool letter-case cases,
  5 Git Bash cases); the doctor tests failed to build (helpers absent). All pass after.
- Criteria links: `tests` → C1–C4. For C4 the tests cover doctor's helpers (hook input JSON, Git sh lookup,
  PATH list), not doctor end to end; for C3 `leadyard run` ran on Windows only. The Linux half of C3, the
  end-to-end doctor (C4) and C5 are observations below, not evidence records.
- Commits on `agent/01M50GXCB1T4XHE0GR59NJ5SQ8-windows` (local, not pushed): 9c9433c fix(gate), 490ed16
  fix(doctor), 56f22cc build, 6345014 ci, 9d558b9 docs. Git has no global identity on this machine; commits
  use the repository's author `zireaelq` with `Assisted-by` (design §7.4, the human's account plus a trailer).

Observations (not evidence records):
- doctor, rebuilt binary (sha256 4d2681ce…), in this repository from Git Bash and from PowerShell (HOME
  unset): every line `ok` except `[warn] git pre-push hook not installed` (`leadyard init` was not run in this
  clone); exit 0 in both. Before: `[FAIL] hook wrapper … missing or not executable`, exit 1 (C4).
- Gate probe, 15 hook inputs through `.claude/hooks/leadyard-gate.sh` under Git's sh: old binary (sha256
  d106ad18…) 13/15, missing `/c/...` path and `.LEADYARD\CONFIG.YAML`; new binary 15/15; the other 13 inputs
  give the same verdicts (answer Q1). The probe "minimal PATH, real HOME" finds `~/go/bin/leadyard`, which
  is the new binary in both runs: it tests the wrapper, not the binary.
- Fresh clone of the branch with `core.autocrlf=true`: 0 files with CRLF in the worktree
  (`git ls-files --eol`), `gofmt -l .` empty, `go test ./skills/` ok (C5). Before: every file CRLF, the
  skills test failed.
- Linux: `docker run golang:1.27.2` (digest sha256:820153096fda…, go1.27.2 linux/amd64, git 2.47.3) on this
  tree mounted read-only: `gofmt -l .` empty, `go vet ./...` clean, `go test -count=1 ./...` ok in every
  package; `TestGitBashPathsOnWindows` skips there by design (C3, Linux half).

Review round 1: `leadyard review input` written to a file; a fresh read-only subagent got only the file's
path and "follow it exactly, modify nothing" — not the text inlined, to avoid retyping 24 KB of diff (a
departure from the review skill's "exactly that file as the prompt"). Verdict warn, 5 advisory findings.
Checked: f-fd6100 reproduced with the new binary — Write `\Users\User\projects\leadyard\.leadyard\config.yaml`
and `\\?\C:\Users\…\.leadyard\config.yaml` → exit 0 (allowed); it was so before this task too. f-467991,
f-b70219, f-13a368 hold on reading the code. Waiting for the human's triage.

Round 1 fixes (human: fix f-fd6100, f-467991, f-b70219, f-13a368; dismiss f-3bffcf; gate_test.go edit
accepted at blob 55922c38):
- New gate tests went to a new file, `internal/gate/paths_test.go`, so the accepted `gate_test.go` stays as
  accepted. Test first: `TestDrivelessPathsOnWindows` failed 5 cases before `windowsPath`; `TestGitSh`
  failed "sh above the install" and "shim" before the layout check; `TestFindHookEnvWindows` and
  `TestFindHookEnvUnix` passed at once (f-13a368 was missing coverage, not a defect).
- Beyond the letter of f-467991: the first lookup probe ("wrapper blocks when leadyard cannot be found")
  also sent a push to main, so its `warn` branch (leadyard found outside PATH) could never fire; it sends
  `git status` too. Same cause, same fix; flagged for the scope audit.
- Decision 68 now also names `\\.\` device paths as not normalised (they were not in the fix).
- Commits 8e64efa fix(gate), 73fdbc5 fix(doctor), 1b5e557 docs. The first push of these three commits had
  `\?\` for `\\?\` in two messages (the shell ate a backslash); they were rewritten before anything left the
  machine with `git reset --soft 9d558b9` and recommitted from message files; the tree is identical.

Observations after the fixes (binary sha256 a1302ab3…):
- doctor from Git Bash and from PowerShell (HOME unset): all `ok` but the pre-push `warn`, exit 0.
- doctor with HOME an empty folder and leadyard not on PATH: `[FAIL] hook cannot find leadyard with a
  minimal PATH (exit 2)` — the check can fail now (f-467991). The pre-task binary never reached it (it failed
  on the exec bit first).
- Gate probe, 17 inputs (two added: `\...` without a drive, `\\?\C:\...`): pre-task binary 13/17, new 17/17.
- Linux, `golang:1.27.2`: gofmt clean, vet clean, every package ok; `TestFindHookEnvUnix` passes there,
  the Windows-only tests skip.

Review round 2 (same delivery as round 1): warn; f-fd6100, f-467991, f-b70219, f-13a368 fixed; 5 new
advisory findings. Human: fix f-08fcb4 and f-ae01c9, record f-87eb97 in decision 68, fix f-c9b86c, dismiss
f-408842, then round 3.

Round 2 fixes:
- f-08fcb4: `TestRootLetterCase` (in `paths_test.go`) passes on Windows before the fix — `filepath.Rel`
  ignores case there — and failed on Linux in `golang:1.27.2` (3 cases allowed) before the lower-case retry
  in `rel()`; it passes on both after.
- f-ae01c9: `TestLeadyardBin` failed to build before `leadyardBin`. Live, doctor with HOME an empty folder and
  leadyard not on PATH: without LEADYARD_BIN the lookup probe FAILs; with LEADYARD_BIN as
  `C:\Users\User\go\bin\leadyard.exe` or `/c/Users/User/go/bin/leadyard.exe` it is ok; the missing-binary
  probe stays ok (it does not get the variable).
- f-c9b86c, f-87eb97: decision 68 rewritten as an open list ("Among others") naming them; decision 67 names
  the root.
- Commits 58f10ed fix(gate), 3b338f7 fix(doctor), 48f274e docs.
- After: `leadyard run` fmt, vet, tests (C1–C4) passed; binary sha256 558588f9…; doctor all `ok` but the
  pre-push `warn`, exit 0; gate probe 17/17; Linux gofmt, vet and every package ok.

## Decisions

<!-- written only by `leadyard decide` and `leadyard transition` -->
- 2026-10-15 19:34:46 · agent · proposed_class = 3 · "Planned files hit zone gate (internal/gate/**, class 3, zones.yaml: 'a mistake lets an agent push or merge'); doctor.go and .gitattributes are outside zones. Five questions: 1 reversible - yes, a local commit, nothing pushed; 2 customer money or data - no; 3 model behavior - no, skills unchanged; 4 contract with another system - no, the hook stdin/exit-code contract with Claude Code is unchanged; 5 security - yes, the gate is the guard on agent writes (findings 2-3 are fail-open denies). Highest: 3."
- 2026-10-15 19:34:46 · agent · transition start = new -> clarifying
- 2026-10-15 19:37:15 · human:redikultsev · class = 3 · "Подтверждаю класс 3"
- 2026-10-15 19:37:15 · human:redikultsev · scope_approved · "Подтверждаю класс 3 и scope C1–C5"
- 2026-10-15 19:37:15 · human:redikultsev · answer Q1 level 4 is unreachable (no level-4 check configured) = accept_with_risk at the verdict; old/new gate probe in Notes · "accept_with_risk на вердикте с пробой старый/новый в Notes"
- 2026-10-15 19:37:15 · human:redikultsev · answer Q2 letter case in gate checks = case-insensitive on every OS · "регистр не различаем на всех ОС"
- 2026-10-15 19:37:15 · human:redikultsev · answer Q3 other Windows path aliases = out of scope; recorded as a known limit · "прочие псевдонимы путей вне задачи"
- 2026-10-15 19:37:16 · human:redikultsev · answer Q4 CI on windows-latest (protected path .github/workflows/**) = add a windows-latest job; protected path change confirmed · "добавить windows-latest в CI"
- 2026-10-15 19:37:16 · human:redikultsev · answer Q5 leadyard run from PowerShell = out of scope · "leadyard run вне задачи"
- 2026-10-15 19:37:16 · human:redikultsev · answer Branch and commits = branch agent/01M50GXCB1T4XHE0GR59NJ5SQ8-windows from main; Conventional Commits with Assisted-by; local only, no push · "Ветка и коммиты — как предло[жено]; ничего не пушь"
- 2026-10-15 19:37:16 · agent · transition plan = clarifying -> planning
- 2026-10-15 19:39:48 · human:redikultsev · plan_approved · "Одобряю план целиком, включая скачивание Docker-образа golang:1.27.2 для проверки на Linux. Приступай к реализации."
- 2026-10-15 19:39:49 · agent · transition implement = planning -> in_progress
- 2026-10-15 19:47:40 · agent · transition verify = in_progress -> verifying
- 2026-10-15 19:58:34 · human:redikultsev · finding_dismissed f-3bffcf · "f-3bffcf отклоняю: всё одобрено или безвредно"
- 2026-10-15 19:58:34 · human:redikultsev · test_edit_accepted internal/gate/gate_test.go = 55922c3840f4459c796ba74a4398754fa870f535 · "Правку gate_test.go принимаю: корень стал родным для ОС, старые случаи сохранены."
- 2026-10-15 19:58:34 · human:redikultsev · answer review round 1 triage = fix f-fd6100, f-467991, f-b70219, f-13a368 as proposed · "Чини f-fd6100, f-467991, f-b70219, f-13a368 как предложено"
- 2026-10-15 20:13:51 · human:redikultsev · finding_dismissed f-408842 · "f-408842 отклоняю: всё одобрено или безвредно"
- 2026-10-15 20:13:51 · human:redikultsev · answer review round 2 triage = fix f-08fcb4 and f-ae01c9; record f-87eb97 in decision 68 as a limit; fix f-c9b86c (among others + list); then round 3 · "Чини f-08fcb4 и f-ae01c9; f-87eb97 запиши в decision 68 как ограничение; f-c9b86c чини («в том числе» + перечень). Потом раунд 3."
- 2026-10-15 20:26:39 · agent · transition ask_human = verifying -> waiting_human · "verdict"
- 2026-10-15 20:38:08 · human:redikultsev · answer scope audit: internal/gate/paths_test.go not in the plan = keep · "paths_test.go и ci.yml оставить"
- 2026-10-15 20:38:08 · human:redikultsev · answer scope audit: protected path .github/workflows/ci.yml = keep · "paths_test.go и ci.yml оставить"
- 2026-10-15 20:38:08 · human:redikultsev · answer scope audit: departures 1-3 (lower-cased path in messages after the root retry; git dir in the Windows minimal PATH; first lookup probe on git status) = keep · "отступления 1–3 оставить"
- 2026-10-15 20:38:08 · human:redikultsev · finding_dismissed f-87eb97 · "f-87eb97 и f-5b23ad отклоняю"
- 2026-10-15 20:38:08 · human:redikultsev · finding_dismissed f-5b23ad · "f-87eb97 и f-5b23ad отклоняю"
- 2026-10-15 20:38:08 · human:redikultsev · answer f-07d371 = follow-up, open · "f-07d371 — follow-up"
- 2026-10-15 20:38:10 · human:redikultsev · transition accept_with_risk = waiting_human -> ready_to_merge · "accept_with_risk: уровень 3, проверки уровня 4 нет."
