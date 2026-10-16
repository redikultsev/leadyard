---
id: 01M50N96K0HFVSMTV7251NAMJ2
title: 'doctor: LEADYARD_BIN from local and user settings'
kind: fix
status: ready_to_merge
class: 2
proposed_class: 2
attempt: 1
base: 863ebdcd606d16c72133e43ed746a3578524d605
created: 2026-10-15T20:50:26Z
---

## Request (verbatim)

Do this task here: “leadyard: LEADYARD_BIN из local/user settings (f-07d371)”

## Acceptance criteria

<!-- quoted from the ticket or the request: "- C1. ..." -->

Derived from review finding f-07d371 of task 01M50GXCB1T4XHE0GR59NJ5SQ8 ("Read local and user settings too
(local > project > user), or say in the FAIL message that only .claude/settings.json is checked").

- C1. doctor's lookup probe gets LEADYARD_BIN with the precedence `.claude/settings.local.json` >
  `.claude/settings.json` > user settings (`$CLAUDE_CONFIG_DIR/settings.json`, else `~/.claude/settings.json`)
  > the environment. (derived)
- C2. The missing-binary probe still gets no LEADYARD_BIN. (derived)
- C3. doctor says where LEADYARD_BIN came from, and the FAIL message names the places it checked. (derived)
- C4. `go test ./...` passes on Windows and on Linux. (derived)

## Plan

<!-- written by the plan step; checkboxes show progress, not acceptance -->

Branch `agent/01M50N96K0HFVSMTV7251NAMJ2-leadyard-bin` from main 863ebdc. Test first.

- [x] 1. Tests in `internal/cli/doctor_test.go`: rewrite `TestLeadyardBin` for the new contract — local >
  project > user settings > environment, the source named, an unreadable file reported and skipped, a file
  without the variable passed over (C1, C3). It is a pre-existing test whose function's contract changes by
  the approved scope (answer Q2): the human is asked to accept the edit. Add `TestSettingsSources`: the user
  file is `$CLAUDE_CONFIG_DIR/settings.json`, else `<home>/.claude/settings.json` (C1).
- [x] 2. Code in `internal/cli/doctor.go`: `settingsSources(root)` and `leadyardBin(sources)` returning the
  value, its source and the unreadable files; the lookup probe gets the value (the missing-binary probe still
  none, C2); its ok line names the source, its FAIL line names the places in order (C3); a `warn` for an
  unreadable local or user settings file. User home by `os.UserHomeDir()` (`%USERPROFILE%` on Windows, as
  the docs say for `~/.claude`).
- [x] 3. `docs/decisions.md`: decision 70 — doctor reads LEADYARD_BIN by Claude Code's precedence; not seen:
  managed settings, `--settings`, a launch environment that wins over settings (Claude Desktop, self-hosted
  runners), `CLAUDE_CONFIG_DIR` set inside a settings file (answer Q3).
- [x] 4. `leadyard run fmt`, `vet`, `tests --criteria C1,C3,C4`; Linux in `golang:1.27.2`; live doctor runs as
  observations: Git Bash and PowerShell, LEADYARD_BIN only in a temporary user settings (`CLAUDE_CONFIG_DIR`),
  only in `.claude/settings.local.json` (created and removed for the run), missing-binary probe (C2).
- [x] 5. Commits on the branch, `Assisted-by: claude-code/claude-opus-5-5`, messages from files; no push.
- Out of scope: managed settings, `--settings`, the Desktop launch environment (Q3, decision 70); the other
  doctor checks and messages.

## Notes

<!-- working notes for resuming after compaction: hypotheses, running jobs, findings -->

Context: follow-up of task 01M50GXCB1T4XHE0GR59NJ5SQ8 (Windows), which the human merged into main locally
(fast-forward to 863ebdc, not pushed) before this task, so its base is clean. The first commit of this
branch (5943609) only records that task's `merged` status. The Claude Code session doing the work was started
in another folder: this repository's hooks do not guard it.

Prior work: `leadyardBin` (internal/cli/doctor.go) reads the env of `.claude/settings.json`, then the process
environment (task 01M50GXCB1T4XHE0GR59NJ5SQ8, finding f-ae01c9). No other task touches it. My reading: not done.

Claude Code docs (checked 2026-10-15 through the claude-code-guide agent; quotes and URLs in this session):
- settings precedence, highest first: managed, command line (`--settings`), `.claude/settings.local.json`,
  `.claude/settings.json`, user `~/.claude/settings.json`; an `env` value follows these levels per variable
  (code.claude.com/docs/en/settings.md, settings-reference.md);
- a settings `env` value replaces the same variable inherited from the shell "in most sessions"; when Claude
  Desktop or a self-hosted runner starts the session, the launch environment wins for a variable it sets
  (code.claude.com/docs/en/env-vars.md, "Precedence");
- `CLAUDE_CONFIG_DIR` moves the user settings ("default: `~/.claude`"); it is ignored in project and local
  settings; on Windows `~/.claude` is `%USERPROFILE%\.claude`;
- managed settings on Windows: `C:\Program Files\ClaudeCode\managed-settings.json`, a `managed-settings.d\`
  folder, and the registry; hooks run in Git Bash on Windows, PowerShell when Git Bash is missing.
So doctor can only approximate what a hook sees; the gaps go to decision 70 (human: out of scope).

Implementation (2026-10-15):
- Pre-existing test edited: `TestLeadyardBin` in `internal/cli/doctor_test.go` (added by task
  01M50GXCB1T4XHE0GR59NJ5SQ8, now in main). Its function's contract changed by the approved scope (answer Q2):
  it took a parsed settings map and returned a string; now it takes the settings sources and returns the
  value, the source and the unreadable files. The test is rewritten for that contract; `TestSettingsSources`
  is new. Needs the human's acceptance.
- Test first: both tests failed to build before `envSource`, `settingsSources` and the new `leadyardBin`;
  both pass after, on Windows and on Linux (`golang:1.27.2`).
- Criteria links: `tests` → C1, C3, C4 (unit tests of the helpers; C4's Linux half is the Docker run below).
  C2 and the doctor lines of C3 are observations below.
- Commits 8d1961e fix(doctor), 11bb183 docs; binary sha256 51f08855….

Observations (not evidence records), doctor with HOME an empty folder and PATH `/usr/bin:/bin:/mingw64/bin`
so that only LEADYARD_BIN can find leadyard:
- nowhere: `[FAIL] hook cannot find leadyard with a minimal PATH (exit 2): … set LEADYARD_BIN in the env of
  .claude/settings.local.json, .claude/settings.json, C:\Users\User\.claude\settings.json (read in this
  order, then the environment)`;
- only in a temporary user settings (`CLAUDE_CONFIG_DIR`): ok, "LEADYARD_BIN from <that settings.json>";
- also in `.claude/settings.local.json` (created for the run, removed after): ok, "from
  .claude/settings.local.json" — local wins;
- `.claude/settings.local.json` not valid JSON, value in user settings: `[warn] …settings.local.json is not
  valid JSON: its LEADYARD_BIN was not read`, then ok from the user settings;
- in all four, "wrapper blocks when leadyard cannot be found" stays ok: the missing-binary probe gets no
  LEADYARD_BIN (C2).
- Normal doctor from Git Bash and PowerShell: all ok but the pre-push `warn`, exit 0.
- Linux `golang:1.27.2`: gofmt clean, vet clean, every package ok.

Review round 1 (fresh read-only subagent, given the bundle's path as in task 01M50GXCB1T4XHE0GR59NJ5SQ8):
warn, 4 advisory. Human: fix f-b31a91 and f-a589f0; f-fc2341 — a read-error test and `tests` linked to C1
and C4 without C3; dismiss f-3afbe7 (the warn was an assumption stated in clarify and accepted).

Round 1 fixes:
- f-b31a91: a present LEADYARD_BIN wins even when empty (the hook gets it empty); the process environment
  is read with LookupEnv for the same reason. The doctor line then says "LEADYARD_BIN is empty in <source>".
- f-a589f0: unreadable sources carry their error; the warn says "<path> cannot be read: <error>"; the project
  file is skipped only when a FAIL above already named it (`projectReported`).
- f-fc2341: `TestLeadyardBin` gains an empty value, an env that is not an object, and a read error (a folder
  at the path), each checked for the path and its error. It failed to build before the fix, passes after
  on Windows and Linux. `tests` re-run with `--criteria C1,C4`: C3 rests on the observations, not on tests.
- `doctor_test.go` was edited again; the human's acceptance of the edit is asked at the verdict.
- Commit c651652; binary sha256 a3177d9b….

Observations after the fixes (HOME an empty folder, PATH `/usr/bin:/bin:/mingw64/bin`, user settings in
a temporary `CLAUDE_CONFIG_DIR` with LEADYARD_BIN set):
- `"LEADYARD_BIN": ""` in `.claude/settings.local.json`: `[FAIL] hook cannot find leadyard with a minimal
  PATH (exit 2; LEADYARD_BIN is empty in .claude/settings.local.json)`, as the hook would fail;
- `"env": "x"` in `.claude/settings.local.json`: `[warn] …settings.local.json cannot be read: json: cannot
  unmarshal string into Go struct field .env of type map[string]interface {}; its LEADYARD_BIN was not
  read`, then ok from the user settings;
- missing-binary probe ok in both (C2); normal doctor all ok but the pre-push warn; the temporary
  `settings.local.json` removed after each run;
- Linux `golang:1.27.2`: gofmt clean, vet clean, 12 packages ok.

Searched: internal/cli/doctor.go, internal/cli/doctor_test.go, .claude/settings.json, the hook wrapper.
Skipped: gate, level, ledger packages (not touched).

## Decisions

<!-- written only by `leadyard decide` and `leadyard transition` -->
- 2026-10-15 20:50:57 · agent · proposed_class = 2 · "Planned files: internal/cli/doctor.go, internal/cli/doctor_test.go, docs/decisions.md - outside zones (gate, evidence). Five questions: 1 reversible - yes, local commits; 2 money or data - no; 3 model behavior - no; 4 contract - no, the hook contract is unchanged; 5 security - no, doctor is a diagnostic, not a guard. Highest: 2 (a regular change)."
- 2026-10-15 20:50:57 · agent · transition start = new -> clarifying
- 2026-10-15 20:50:57 · human:redikultsev · class = 2 · "Подтверждаю класс 2"
- 2026-10-15 20:50:58 · human:redikultsev · scope_approved · "Подтверждаю класс 2 и scope C1–C4."
- 2026-10-15 20:50:58 · human:redikultsev · answer Q0 base: merge the Windows branch into main = merged locally, fast-forward, no push · "Сливай ветку Windows в main локально ff-only, без push."
- 2026-10-15 20:50:58 · human:redikultsev · answer Q2 sources of LEADYARD_BIN = local > project > user settings (CLAUDE_CONFIG_DIR), then the environment; doctor names the source; FAIL lists the places · "Вопросы 2 и 3 — вариант (а)"
- 2026-10-15 20:50:58 · human:redikultsev · answer Q3 managed settings, --settings, Desktop launch environment = out of scope; decision 70 records them as limits · "managed и запуск из Desktop в decision 70 как ограничение."
- 2026-10-15 20:50:58 · agent · transition implement = clarifying -> in_progress
- 2026-10-15 20:55:36 · agent · transition verify = in_progress -> verifying
- 2026-10-15 21:02:58 · human:redikultsev · finding_dismissed f-3afbe7 · "f-3afbe7 отклоняю: это принятое допущение."
- 2026-10-15 21:02:58 · human:redikultsev · answer review round 1 triage = fix f-b31a91 and f-a589f0; f-fc2341: a read-error test, tests linked to C1 and C4 without C3; then round 2 · "Чини f-b31a91 и f-a589f0; f-fc2341 — тест на ошибку чтения и tests с критериями C1, C4 без C3. Потом раунд 2."
- 2026-10-15 21:10:11 · agent · transition ask_human = verifying -> waiting_human · "verdict"
- 2026-10-16 08:12:12 · human:redikultsev · test_edit_accepted internal/cli/doctor_test.go = a9cde258feffcf7110b84677eb3af1111f32d660 · "Правку doctor_test.go принимаю: TestLeadyardBin переписан под новый контракт."
- 2026-10-16 08:12:12 · human:redikultsev · finding_dismissed f-359355 · "f-359355 отклоняю: строки doctor остаются наблюдением, как решено."
- 2026-10-16 08:12:14 · human:redikultsev · transition approve_verdict = waiting_human -> ready_to_merge · "approve_verdict"
