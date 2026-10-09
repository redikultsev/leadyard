# leadyard

An open framework for the daily loop of a developer and a tech lead working with coding agents.

- **"Done" is evidence, not a claim.** A task has a level from 0 to 5, computed by the CLI from
  check runs with links. Evidence goes stale when the covered files change.
- **Rules that matter are enforced.** A pre-tool hook parses every shell command (so
  `git -C . push origin main` is seen as a push) and blocks what the git policy forbids; a git
  `pre-push` hook repeats the critical denies.
- **A human decides** the verdict, the merge and the deploy. The risk class decides the rest.
- **State lives in files in git**, readable by people and any agent.

Status: **0.1, early.** Runs the developer loop end to end on Claude Code. Codex and Cursor come in
0.2. Versions before 1.0 follow the `0.y.z` rule: anything may change. `leadyard` is a working name.
Design: [docs/design.md](docs/design.md) · decisions: [docs/decisions.md](docs/decisions.md).

## Install

Requires git and Go 1.26 (Go 1.21 or newer downloads the right toolchain by itself).

```bash
go install github.com/redikultsev/leadyard/cmd/leadyard@latest
```

From a clone: `go install ./cmd/leadyard`. Make sure `$(go env GOPATH)/bin` is on your `PATH`:
the hooks call `leadyard`, and without it they block guarded actions.

## Quick start

In the repository you work on:

```bash
leadyard init      # config, zones, Claude Code hooks and skills, git hooks
```

Then edit `.leadyard/config.yaml`: map your commands to levels.

```yaml
git:
  delivery: none        # none | local | branch | pr — none: the agent never writes git
checks:
  - {id: lint,  level: 1, run: "make lint"}
  - {id: tests, level: 3, run: "make test", tests: ["**/*_test.go"]}
```

```bash
leadyard doctor    # checks the install, including that the hook blocks without the binary
```

Commit the files `init` created. Start Claude Code and give it a task; the `leadyard` skill drives
the loop. You answer questions in chat; the agent records your answers.

To keep git writes to yourself in every repository, put this in `~/.config/leadyard/config.yaml`:

```yaml
git: {delivery: none}
```

A user setting can only tighten the git policy, never widen what a team allowed.

## The loop

| Stage | What happens | Commands |
|---|---|---|
| intake | request verbatim, acceptance criteria, facts, proposed class | `task new`, `decide proposed_class`, `zones` |
| clarify | one round of questions; you confirm the class and the scope | `decide class`, `decide scope_approved` |
| plan | class 3 only; you approve it | `decide plan_approved` |
| implement | test first; checks run through the CLI | `run <check> --criteria C1` |
| review | fresh session gets a bundle the CLI builds | `review input`, `review record` |
| scope audit | final diff against the plan, protected paths, zones | `scope` |
| verdict | level, criteria, open findings, what was not verified | `level`, `transition approve_verdict` |
| PR | draft description from the evidence | `pr-body` |

Levels: 1 static checks · 2 agent tests · 3 the project's existing tests plus a fresh-session review
without open blocking findings · 4 old vs new on real cases · 5 after deploy (later). Required level by
class: 1 → 1, 2 → 3, 3 → 4.

After a context compaction Claude Code runs `leadyard resume`, which prints the task state.

## What it does not do (yet)

Records are tamper-evident, not tamper-proof: the agent records your answers, and anything with
write access to the repository can change the files. Merges and deploys stay with you. See
design §24 for what is deferred.

## License

MIT
