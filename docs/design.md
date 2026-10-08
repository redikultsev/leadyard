# leadyard — design

Status: draft 0.1, 2026-10-08. `leadyard` is a working name.

This document describes what leadyard is, how it is built, and why. Decisions are listed with
their reasons in [decisions.md](decisions.md). Section numbers here are stable; other documents
link to them.

---

## 1. Problem

Coding agents write code quickly. The time they save moves elsewhere:

- **"Is it really done?"** The agent says "done". The human cannot tell whether that means
  "compiles", "tests pass", or "checked against real data", so the human asks again and again,
  or checks by hand. A controlled study found experienced developers were 19% slower with AI
  tools while believing they were 20% faster
  ([METR, July 2025](https://metr.org/blog/2025-07-10-early-2025-ai-experienced-os-dev-study/)).
  Self-reported "done" and self-reported speed are both unreliable.
- **Rules are written but not enforced.** Teams put "never push to main", "keep the diff
  minimal", "do not hardcode" into `AGENTS.md`. Agents read these as context, not as limits.
  The same corrections repeat in every task.
- **Context is lost.** Long tasks span context compactions and sessions. People copy
  "how to continue" notes by hand.
- **The same rituals are repeated by hand:** review → triage findings → fix → re-run tests;
  "merge main, resolve conflicts, check again"; "bring up the local stand and test";
  "after deploy, check the logs".
- **The tech lead sees claims, not evidence.** Status updates, risk, promises and review queues
  are assembled from what people and agents say.

leadyard turns this loop into a defined process with machine-checked evidence, mechanical
guardrails, and human decisions at a small number of fixed points.

## 2. Goals and non-goals

**Goals**

1. One working loop for a developer and a tech lead, usable with any coding agent.
2. "Done" is a level of evidence computed from artifacts, not a statement.
3. Rules that matter are enforced by mechanisms and verified by probes.
4. A human always decides the verdict, the merge to the main branch, and the deploy.
5. Everything that must come out the same every time is computed by a program, not a model.
6. State lives in files in git. Any agent and any human can read it.
7. Every component is replaceable: skills, trackers, MCP servers, runners.

**Non-goals**

- A dashboard or control panel. Trackers, git hosting and the IDE already show work.
- An orchestrator that runs a fleet of agents. leadyard prepares the agent, checks its work,
  and records decisions. A human or CI starts the agent.
- Autonomous merge or deploy.
- A memory store. Durable knowledge is files in the repository.
- Sending any data anywhere. Telemetry is a local event log.

## 3. Principles

1. **Evidence over claims.** A level counts only with an artifact link that the program can
   verify. Agent words are level 0.
2. **Mechanism over instruction.** If a rule must hold, a hook, git, CI or the server enforces
   it. Instructions are context.
3. **Fail closed.** A missing artifact is level 0. An unparseable verdict is `fail`. A crashed
   guard blocks.
4. **The program computes, the model proposes.** Levels, staleness, risk class aggregation,
   statistics, transitions and config writes are done by the CLI.
5. **One writer per artifact.** Every file has exactly one stage that writes it. Others read.
6. **Nothing is enabled silently.** Every module, MCP server and agent capability is turned on
   by an explicit action, and the actual state is printed.
7. **Ask only at gates.** Human attention is the scarce resource. Questions are batched and
   asked at defined points.
8. **Every compensating mechanism names the model weakness it compensates.** When models
   change, mechanisms are re-evaluated and removed if no longer needed.

## 4. Architecture

leadyard has three layers and a set of adapters.

```
┌──────────────────────────────────────────────────────────────────────┐
│ 1. Spec            formats and schemas: task file, evidence record,  │
│                    verdict block, transitions, config, zones         │
│                    (public API, semver)                              │
├──────────────────────────────────────────────────────────────────────┤
│ 2. Reference       stage skills (default pack + own), hooks,         │
│    skills & hooks  templates                                         │
├──────────────────────────────────────────────────────────────────────┤
│ 3. CLI             `leadyard`, one Go binary: levels, staleness,     │
│                    gates, transitions, probes, config, resume        │
└──────────────────────────────────────────────────────────────────────┘
          │ generates                                  ▲ called by
          ▼                                            │
┌──────────────────────────────────────────────────────────────────────┐
│ Adapters   per agent: instruction files, skill placement, hook       │
│            wiring, permission rules, MCP config, probes              │
│            Claude Code · Codex · Cursor · (experimental) ...         │
└──────────────────────────────────────────────────────────────────────┘
```

- The **spec** is the contract. Anyone can implement a compatible skill, adapter or tool
  against it.
- The **skills** do the work that needs a model: understanding a request, planning, writing
  code, reviewing.
- The **CLI** does everything that must be deterministic. Skills and hooks call it. It is a
  single static binary so that a hook can call it on every tool use without a runtime.
- **Adapters** are generators. From one set of framework files they produce the configuration
  each agent understands. The CLI is the only writer of agent configuration files.

## 5. Core concepts

### 5.1 Task

A task is the unit of work. Its state is split between two places:

| Where | What | Who reads it |
|---|---|---|
| Tracker (GitHub Issues, Jira, Linear, or none) | what to do, who, status for the team | people |
| Task folder (in git) | original request, decisions, plan, evidence, agent state | people and agents |

A tracker is optional. A sync adapter mirrors status between the two (§11).

Task folder layout:

```
.leadyard/tasks/<task-id>/
  task.md          front matter (machine fields) + request, decisions, plan
  evidence.jsonl   evidence records, append-only, written only by the CLI
  events.jsonl     event log, append-only, written only by the CLI
  artifacts/       reports, verdict blocks, run outputs referenced by evidence
```

`task.md`:

```markdown
---
id: T-0142
tracker: https://github.com/org/repo/issues/142
status: in_progress            # §5.2, changed only by `leadyard transition`
class: 2                       # §5.5, proposed by agent, confirmed by human
delivery: pr                   # §7.2
owner: agent                   # agent | human
owner_reason: null
attempt: 3
blocked: null                  # {reason, retry_after, return_to}
hold: false
start_not_before: null
limits: {review_rounds: 5, retries: 5}   # §5.8, raised only by a recorded human decision
---

## Request (verbatim)
<the original request, copied as is>

## Decisions
<!-- append-only; written by `leadyard decide` -->
- 2026-10-08 14:02 · human:alice · scope approved · "only the date filter, no sorting"

## Plan
<!-- written by the plan stage; checkboxes ticked by the implement stage -->
- [x] 1. Add `date_from` / `date_to` to the query schema
- [ ] 2. ...
```

### 5.2 Statuses

Statuses are fixed in the core so that reports across projects add up. A project may hide
statuses it does not use. It may not add its own.

| Status | Meaning |
|---|---|
| `new` | accepted, nothing done yet |
| `clarifying` | intake and questions |
| `planning` | plan being written or awaiting approval |
| `in_progress` | implementation |
| `verifying` | scope audit, review, verdict evidence being collected |
| `waiting_human` | a human decision is required; reason recorded |
| `ready_to_merge` | verdict accepted; merge package ready |
| `merged` | merged by a human |
| `deployed` | deployed by a human |
| `closed` | done; post-deploy watch finished or not required |
| `on_hold` | paused by a human |
| `cancelled` | will not be done |

### 5.3 Transitions

Transitions are data in the core (`spec/transitions.yaml`). Each row is: event, allowed source
statuses, who may trigger it, target status, side effects.

```yaml
- event: approve_verdict
  from: [waiting_human]
  actor: human
  requires: {level_at_least: class_required_level}
  to: ready_to_merge
- event: request_changes
  from: [waiting_human, ready_to_merge]
  actor: human
  requires: {reason: true}
  to: in_progress
  effects: [new_attempt]        # §5.4: evidence of the previous attempt becomes stale
```

Rules:

- `leadyard transition <event>` is the only way to change `status`. It refuses with a
  structured code (`not_allowed_from`, `evidence_missing`, `reason_required`, `actor_mismatch`).
- Skills read the table to know which answers they may offer a human. They do not invent them.
- A project may **tighten** the table (add a required human approval). It may **loosen** it
  only explicitly, with a recorded reason.
- Returning work requires a reason. All reasons are kept in the Decisions log, not only the
  last one.

### 5.4 Evidence ladder

"Done" is a level from 0 to 5.

| Level | Name | Example |
|---|---|---|
| 0 | claim | the agent says "done, works" |
| 1 | static | lint, type check, build passed; link to the log |
| 2 | agent tests | tests written for this task pass |
| 3 | independent check | the project's existing test suite passes, or a reviewer in a fresh session without the implementation context found no blocking issues |
| 4 | realistic data | old and new versions compared on real or recorded cases (§5.6) |
| 5 | post-deploy | after deploy, the watch window passed without regressions |

The project maps its own checks to levels in config, for example "`npm test` is level 3,
the replay suite is level 4" (§10).

**Evidence record** (one line in `evidence.jsonl`, written only by the CLI):

```json
{
  "id": "ev-0007",
  "task": "T-0142",
  "attempt": 3,
  "check": "unit-tests",
  "level": 3,
  "status": "passed",
  "artifact": "https://ci.example.com/runs/88123",
  "commit": "4f1c2e9",
  "tree_digest": "sha256:9a1b...",
  "covers": {"src/api/query.go": "sha256:...", "src/api/query_test.go": "sha256:..."},
  "snapshot": {"before": "sha256:...", "after": "sha256:..."},
  "recorded_at": "2026-10-08T14:20:11Z"
}
```

Record status is one of: `passed`, `failed`, `missing`, `stale`, `invalid`, `incomplete`,
`skipped`.

Rules the CLI applies:

1. **The task level** is the highest level that has a `passed`, fresh record from the current
   attempt. An empty set is level 0.
2. **Staleness.** A record covers a map of file paths to SHA-256 hashes. If any covered file
   changes, is added, removed or renamed, the record becomes `stale`. If the coverage is
   unknown, any change in the repository makes it stale.
3. **Snapshot before and after.** The CLI hashes inputs before a run starts and again after it
   ends. If they differ, the result is `incomplete`. A fresh hash is never attached to an old
   success.
4. **Attempts.** Each attempt has a number. A late result from an earlier attempt cannot raise
   the level. Starting a new attempt marks earlier records stale in one operation.
5. **Artifact check.** `leadyard evidence add` verifies that the artifact exists and reports
   success (CI run status, exit code in a saved log, report schema). Otherwise it refuses.
6. **Structure check is separate.** `leadyard check --structure` validates formats and never
   counts levels, so producing new evidence never requires presenting old evidence.
7. **Negative control.** Every check type ships with a fixture on which it must fail. The
   framework's own tests run these fixtures.

**Level 3 definition.** An independent check is a check that the implementing session cannot
influence: the project's pre-existing test suite, or a review performed in a fresh session that
receives the diff and the request but not the implementation conversation.

### 5.5 Risk classes

A task has a risk class from 1 to 3. The class decides which stages run, which gates apply,
and which level is required.

The class comes from two sources:

1. **Zones.** The project describes once, in `.leadyard/zones.yaml`, the areas where a change is
   sensitive and why. A zone can be defined by paths and by kind of behavior.

   ```yaml
   zones:
     - id: model-behavior
       class: 3
       why: "prompt or threshold changes alter outputs that customers pay for"
       paths: ["prompts/**", "src/agent/tools/*.py", "config/thresholds.yaml"]
       requires: {evidence: [level-4-compare], docs: ["docs/prompts.md"]}
     - id: public-api
       class: 2
       paths: ["api/openapi.yaml"]
   ```

2. **Five fixed questions,** answered on every task by the agent:
   1. Is the change reversible?
   2. Does it touch customer money or data?
   3. Does it change model behavior or another non-deterministic part?
   4. Does it change a contract with another system?
   5. Does it touch security?

The class is the highest class produced by any zone the diff touches and any answer. A missing
answer raises the class. The agent proposes the class; a human confirms it. `leadyard zones
check` computes the zone part from the diff, not the model.

Defaults per class:

| | Class 1 | Class 2 | Class 3 |
|---|---|---|---|
| Example | docs, rename | regular change | money, data, security, irreversible, model behavior |
| Level before merge | 1 | 3 | 4 |
| Level before close | 1 | 3 | 5 |
| Human approves scope | — | yes | yes |
| Human approves plan | — | — | yes |

A project may raise these values. It may lower them only explicitly, with a recorded reason.

### 5.6 Verdict for non-deterministic systems

When outputs vary between runs (LLM features, ML, flaky tests, performance), "green / red"
does not work. The core defines one method for all projects:

1. **Noise baseline:** run the old version against itself on the same cases.
2. **Comparison:** run the new version against the old one on the same cases.
3. **Four outcomes:**
   - `worse`: the difference exceeds the noise in the bad direction;
   - `not_worse`: the difference is within the declared tolerance;
   - `better`: the difference exceeds the noise in the good direction;
   - `undetermined`: not enough data. The CLI reports the smallest difference it could have
     detected with this sample.

The project supplies the runner (any command that outputs per-case results in the spec format).
The CLI computes the statistics. The method stores the baseline with the number of runs and
the spread, so that a threshold is never a single measurement.

### 5.7 Gates and human decisions

Always human, in every class:

- accepting the verdict;
- merging into the main branch;
- deploying.

By class (§5.5): approving the scope, approving the plan.

Gates are answered in chat. The answer is recorded in the task file by `leadyard decide`: who,
what, when, reason, links. An answer that is not recorded did not happen.

### 5.8 Stage contract

Every stage has a contract in `spec/stages/<stage>.yaml`:

- inputs (artifacts it reads);
- outputs (artifacts it writes; each artifact has exactly one writer stage);
- outcome, one of:

  | Outcome | Meaning |
  |---|---|
  | `ok` | outputs written |
  | `skipped` | nothing to do; no model call needed |
  | `retry_later` | external cause (rate limit, network, auth); with a time |
  | `needs_human` | with a reason from a closed list |
  | `failed` | the stage could not do its job |

- the verdict block, if the stage judges something (§5.8.1).

Errors are classified by structural signals (exit codes, HTTP status, error type), not by
matching message text. Some causes never retry automatically: branch drift, failed commit,
context overflow, model refusal. They go straight to `needs_human`.

**Limits.** Review rounds and automatic retries default to 5 each. After that the task goes to
`waiting_human` with a summary of what remains. A human may raise a limit for one task; the
decision is recorded.

#### 5.8.1 Verdict block

A stage that judges ends its output with one fenced block. The CLI reads the last block and
validates it against the schema. Anything else in the text is ignored for gating.

````markdown
```leadyard-verdict
{
  "schema": 1,
  "stage": "review",
  "status": "fail",
  "findings": [
    {"id": "f-3c9a", "severity": "blocking", "file": "src/api/query.go", "line": 88,
     "summary": "date_to is exclusive in SQL but inclusive in the docs"},
    {"id": "f-71d0", "severity": "advisory", "summary": "test name does not match behavior"}
  ],
  "previous": [{"id": "f-2b11", "state": "fixed"}],
  "next": ["implement"]
}
```
````

- `status` is `pass`, `warn` or `fail`. A missing or unparseable block is `fail`.
- A finding `id` is computed by the CLI as a hash of the normalized file and summary, so it
  stays stable across rounds. Ids written by the model are ignored: in a probe the reviewer
  numbered findings `F1`, `F2`, which would not survive a second round.
- A repeat review must report the state of every previous finding (`fixed`, `open`,
  `replaced`).
- `next` comes from a closed list in the stage contract.
- **Convergence rule:** if all previous findings are closed but new blocking ones appear, the
  task goes to a human instead of another automatic round.

### 5.9 Modes

Every reference skill supports two modes, selected by `LEADYARD_MODE`:

- `interactive` (default): the skill may ask the human in chat.
- `unattended` (CI, scheduled runs): the skill never asks. If data is missing, it stops with
  `needs_human` and the reason.

## 6. Developer loop

### 6.1 Stages

```
intake → clarify → plan → implement → scope audit → review → verdict → stand → merge package → watch
```

| Stage | Purpose | Class 1 | Class 2 | Class 3 |
|---|---|---|---|---|
| intake | facts, cases, where in code, is it already done | yes | yes | yes |
| clarify | questions to the human, batched, with recommendations | — | yes | yes |
| plan | plan with the request verbatim and checkboxes | — | — | yes |
| implement | code and tests | yes | yes | yes |
| scope audit | diff against request, plan and decisions | yes | yes | yes |
| review | separate reviewer, fresh session, diff only | — | yes | yes |
| verdict | level computed, human accepts or returns | yes | yes | yes |
| stand | run on a local or test environment, realistic data | — | per project | yes |
| merge package | PR body, evidence, deploy order, test notes | yes | yes | yes |
| watch | post-deploy checks over a window | — | per project | yes |

The scope audit runs in every class because unrequested additions (flags, env vars, files,
hardcoded values) are the most frequent correction humans make.

### 6.2 What each stage does

**Intake.** Reads the tracker item and comments, finds the relevant code, looks for prior work
(branches, closed tasks), collects real cases and their frequency when the project provides a
data source. Writes the task file with the request verbatim. Proposes the risk class.
- The search scope is a fixed list in the stage contract: the tracker item and linked items,
  code search for the terms of the request, branches and tasks closed in the last N days, the
  project's case source if configured. The output lists what was searched and what was skipped
  with a reason.
- "Is it already done?" is answered with raw links (branch, commit, task). Any conclusion drawn
  from them is labeled as the agent's reading.
- Each of the five risk answers quotes the line of the request or diff it rests on. The zone part
  of the class comes from `leadyard zones check`, not from the agent.
- The stage runs under a budget from config (tool calls and wall-clock time, counted by the CLI).

**Clarify.** Asks only what blocks the next stage. One round contains every question whose
prerequisites are settled. Each question has options and a recommendation. Answers are recorded
as decisions. The same round lists the assumptions the agent made without asking, so the human
sees what was decided silently.

**Plan.** Writes numbered steps with checkboxes and an explicit "out of scope" list. Every
significant rule in the plan has a source (request, decision, code). An unresolved conflict stops
the stage with `needs_human`.

**Implement.** Works through the plan and ticks a checkbox after each step. A checkbox shows
progress; it is not acceptance. Acceptance comes only from evidence (§5.4). Commits on the task
branch (§7).
- For a fix: first a test that fails, then the change, then the same test passes. The CLI checks
  this: a `failed` record for the test at commit A, a `passed` record at a later commit B, and the
  test file unchanged between A and B. A test edited in between means it was adjusted to the
  implementation; the pair does not count.
- The stage runs under a budget from config. At the budget checkpoint the agent stops and
  answers one question in the task file: what tool or approach would turn the remaining work
  into minutes. Any caveat in the agent's report ("done, except ...") is listed to the human as an
  open item.

**Scope audit.** Deterministic part (CLI): files in the diff that the plan does not mention,
new config keys, env vars, feature flags, dependencies, changes in protected paths, zones
touched. Model part: hardcoded values, departures from recorded decisions. The model part runs in
a fresh session, like the review, so the implementer does not audit itself. Output: a list with
"remove / keep with reason", answered by the human.

**Review.** Runs in a fresh session with the diff, the request and the plan, without the
implementation conversation. The reviewer reports uncertain findings with a confidence mark;
a separate read-only check resolves them before the verdict block. Read-only is enforced by the
environment (permissions), not by the prompt.

**Verdict.** The CLI computes the level, lists what is not verified, and checks the class
requirement. Below the requirement, the human is not asked. At or above it, the human gets:
level with links, findings left open, what was not checked, the scope audit result.

**Stand.** Runs scenarios derived from the diff plus regression scenarios on a local or test
environment.
- Before the run, the agent writes the scenario list with the expected outcome of each and a list
  of scenarios it left out with the reason. The CLI hashes this list into the evidence record, so
  expectations cannot change after the results are seen.
- Pass or fail comes from the runner (exit code, assertions, or §5.6 for non-deterministic
  systems), not from the agent reading the output.
- Where the data comes from is declared in config as a check (`kind: compare` or a scenario
  runner). If no data source is configured, the stage says so and does not invent cases.
- For class 3 the human approves the scenario list together with the plan, in one approval.
- Results are tied to the commit and the tree digest.

**Merge package.** Builds the draft PR body from the task: closes the tracker item, what
changed, how it was verified, what was not verified, deploy order, notes for QA. Every
"verified" line comes from an evidence record with its link; text written by the agent (deploy
notes, QA notes) is marked as such. Checks the changelog rule of the project.

**Watch.** After the human reports a deploy: runs the queries the project configured for logs,
metrics and events over a window set in config, and compares before and after with the method of
§5.6. Level 5 is recorded only from that comparison (`not_worse` or `better`); a regression is
recorded with links. The agent writes a summary of what changed, marked as its reading.

### 6.3 Default skills

The default skill pack is [mattpocock/skills](https://github.com/mattpocock/skills) (MIT),
pinned by version and hash (§14). Stages it does not cover get leadyard's own skills.

| Stage | Default skill | Source |
|---|---|---|
| intake | `leadyard-intake` | own |
| clarify | `grilling` / `grill-with-docs` | mattpocock |
| plan | `to-spec`, `to-tickets` for multi-session work | mattpocock |
| implement | `implement` (drives `tdd`) | mattpocock |
| scope audit | `leadyard-scope` | own |
| review | `code-review` | mattpocock |
| verdict | `leadyard-verdict` | own |
| stand | `leadyard-stand` | own |
| merge package | `leadyard-pr` | own |
| watch | `leadyard-watch` | own |

Any skill can be replaced in config if the replacement satisfies the stage contract: it reads
the declared inputs and writes the declared outputs and verdict block. The mapping above must be
re-checked against the pinned version of the pack before release (§20).

## 7. Git policy

### 7.1 Three lists

**The agent does on its own** (within the delivery mode):

- read: `status`, `diff`, `log`, `blame`, `show`, `fetch`;
- create its task branch;
- `add` explicit paths and `commit`;
- amend or rebase its own unpublished commits;
- push its own branch, without force;
- open and update a draft PR;
- answer review comments with new commits on the same branch.

**With human confirmation, each time:**

- create a separate worktree (parallel tasks, §7.3);
- rewrite published history of its own branch, `--force-with-lease` on its own branch only;
- `reset --hard`, `checkout -- .`, `restore .`, `clean -fd`, `stash drop`, removing a worktree
  that has work;
- marking a draft PR ready for review;
- changing protected paths (CI workflows, CODEOWNERS, agent configuration, dependency manifests
  as the project decides);
- any action beyond the declared delivery mode, or in another repository.

**Never:**

- push to the default or release branches;
- merge or approve any PR;
- `--force` without lease, `--mirror`, `--delete`, deleting remote branches or tags, creating
  tags or releases;
- changing branch protection, rulesets, repository settings, secrets, webhooks;
- `git config`, `git remote add/set-url`, git hooks, `core.hooksPath`, `.git/**`,
  `--no-verify`;
- `Signed-off-by` on behalf of a human, signing with a human key, faking author or committer.

### 7.2 Delivery modes

| Mode | The agent may |
|---|---|
| `local` | commit locally |
| `branch` | plus push its branch |
| `pr` | plus open a draft PR (default) |

A task may narrow the mode. Anything beyond it requires confirmation.

### 7.3 Branches and worktrees

- **Branch name follows the team's convention.** The pattern comes from config. If it is not set,
  the CLI infers it from existing branches and asks once to confirm. Fallback:
  `agent/<task-id>-<slug>`.
- **One task per working tree.** Uncommitted changes block the start of another task in the
  same tree. Parallel work happens only in separate worktrees, created with confirmation.
- **The branch is the source of truth.** Before a stage the CLI restores the expected branch;
  after the stage it checks that the branch did not change. Drift goes to `needs_human`.
- **After implementation the CLI checks, not the agent:** the tree is clean, the branch is the
  same, new commits exist and carry the trailer.

### 7.4 Identity

Two supported setups:

- a separate bot account or app for the agent, without administration rights (recommended for
  teams: then the server itself refuses writes to protected branches);
- the human's account plus a trailer.

One trailer for all agents: `Assisted-by: <agent>/<model>`. The agent's own co-author trailer is
disabled to avoid duplicates. A project switch `ai_contributions: forbidden` makes the agent
produce a patch or text instead of writing to git.

### 7.5 Enforcement layers

| Layer | What enforces | Who sets it up |
|---|---|---|
| Server | branch protection / rulesets: PR required on default and release branches, no force push, no deletion; agent not in bypass | human admin |
| Credentials | agent token without administration; scoped to listed repositories | human admin |
| Environment | container or sandbox; `.git/hooks` and `.git/config` not writable | project |
| Agent | permission rules and pre-tool hook (§8) | leadyard |
| Git | `commit-msg` hook adds the trailer; `pre-push` refuses forbidden targets | leadyard |
| CI | required checks: trailer, PR size, evidence present in the PR | human admin |

What cannot be enforced mechanically, and is stated as such: whether commits are meaningful,
whether the PR description is accurate. Review and evidence cover those.

### 7.6 PR feedback

The CLI reads PR state and turns it into transitions: changes requested → back to
`in_progress` on the same branch; merged → `merged`; closed without merge → `on_hold`.
Repeated runs are idempotent by review id. The verdict comment on the PR is a single comment
updated only when its content changes.

## 8. Guardrails and probes

### 8.1 Baseline

A set of deny rules is on by default (the "never" list of §7.1, destructive filesystem
commands outside the workspace, reading secret files). A project may tighten freely. It may
loosen a rule only explicitly, with a recorded reason.

### 8.2 How a rule is enforced

1. **Pre-tool hook** calls `leadyard gate`, which parses the command (not a string prefix) and
   answers allow or deny. A text pattern like `git push *` misses `git -C . push`; the gate
   parses arguments instead.
2. **The hook fails closed.** Agents that follow the Claude Code hook protocol block only on
   exit code 2 and let the action through on any other code. `leadyard gate` exits with 2 on any
   internal error.
3. **Modes that skip permission prompts are handled.** In Claude Code a pre-tool hook that denies
   blocks even in bypass mode; auto mode can push without asking, so the adapter disables it
   where the project requires (`permissions.disableAutoMode`).
4. **Messages tell the agent what to do instead,** for example "push to main is not allowed;
   push your branch `feat/T-0142-date-filter` and open a draft PR".

### 8.3 Guarantee levels and probes

Each guarantee declares the minimum level at which it must hold:

| Level | Meaning |
|---|---|
| E | the agent harness blocks it |
| V | git hooks or CI block it before merge |
| A | an instruction only |
| H | a human checks |

Each adapter declares what level it can reach. `leadyard doctor` runs probes in headless mode:
it tries a forbidden action and checks that it was blocked, checks that a hook was called, that
an MCP tool is visible, that instructions were loaded. The capability table is generated from
probe results, never written by hand. If a reachable level is lower than the required one,
`doctor`, CI and the PR body say so. There is no silent degradation.

## 9. Agent adapters

### 9.1 Support tiers

| Tier | Agents | Meaning |
|---|---|---|
| certified | Claude Code, Codex, Cursor | live agent runs in CI on a schedule; all probes pass |
| experimental | Gemini CLI, Copilot CLI, OpenCode | probes exist; no scheduled live runs; off by default |
| instructions only | others | `AGENTS.md` only |

An agent moves up after its probes pass on a pinned version, with a dated record.

### 9.2 What the core relies on

Only what nearly every agent supports: `AGENTS.md`, `SKILL.md` with portable front matter
fields (`name`, `description`, `license`, `compatibility`, `metadata`), shell commands, MCP over
stdio and streamable HTTP. Everything richer lives in adapters.

Every core tool is available two ways: as an MCP tool and as a CLI command with `--json`. An
agent without MCP uses the CLI.

### 9.3 Adapter manifest

```yaml
id: claude-code
tier: certified
min_version: "2.1.277"
modes: [interactive, headless]
capabilities:              # filled from probe results, per mode
  instructions: native-via-claude-md
  skills: copy-to-.claude/skills
  pre_tool_hook: E
  permission_rules: E
  mcp: [stdio, http]
  resume_after_compaction: session-start-hook
```

Capabilities depend on the pair (agent, launch mode), not on the agent alone. The CLI takes the
version from the exact binary that will run. "Unknown" is its own status, not success.

### 9.4 Known adapter facts (Claude Code)

- It reads `AGENTS.md` only when there is no `CLAUDE.md`. The adapter writes `CLAUDE.md`
  containing `@AGENTS.md`.
- It does not load skills from `.agents/skills/`. The adapter places skills in `.claude/skills/`.
- After context compaction, a `SessionStart` hook with the `compact` matcher runs
  `leadyard resume`, which prints the task state. Skills write state as they go; the hook only
  re-injects it.

### 9.5 Generation rules

- The CLI is the only generator of agent configuration. Hand-written agent configs outside the
  generator fail a CI check.
- Configs hold names of environment variables, never secret values.
- Headless runs (probes, unattended stages) start with a minimal environment and an explicit
  list of passed variables.

## 10. Configuration

### 10.1 Files and layers

| Layer | File | In git |
|---|---|---|
| distribution | inside the leadyard release | — |
| team | `.leadyard/config.yaml` | yes |
| personal | `.leadyard/config.local.yaml` | no |

Later layers override earlier ones. Merge rules: scalars replace; maps merge; lists of items
with `id` merge by `id`; other lists append; removing a distribution item is an explicit
`disable:` entry. The config has a JSON Schema for editor hints.

The config is written only by `leadyard config set`, which keeps comments and unknown keys.
Agents never edit it as free text.

### 10.2 Example

```yaml
# .leadyard/config.yaml
schema_version: 1
leadyard: ">=0.1 <0.2"
agents: [claude-code, codex]

checks:                      # project checks mapped to levels (§5.4)
  - {id: lint,        level: 1, run: "make lint"}
  - {id: unit-tests,  level: 3, run: "make test", covers: ["src/**", "tests/**"]}
  - {id: replay,      level: 4, run: "make replay", kind: compare}   # §5.6

stages:                      # stage → skill; any skill meeting the contract
  clarify: {skill: grilling}
  review:  {skill: code-review}

git:
  branch_pattern: "feat/{task}-{slug}"   # inferred and confirmed if absent
  delivery: pr
  identity: human-with-trailer
  ai_contributions: allowed
  protected_paths: [".github/workflows/**", "CODEOWNERS", ".leadyard/**"]

limits: {review_rounds: 5, retries: 5}

style:                       # answer style (§15)
  length: short
  show_before_after: true
  changelog_line_max_words: 15

modules:
  - {id: tracker-github, version: "^0.1"}
mcp:
  observability: {enabled: false}        # enabled only explicitly
```

## 11. Workspace and tracker

- **Single repository:** task folders live in the repository (`.leadyard/tasks/`).
- **Several repositories per task, and the tech lead loop:** a team workspace, which is a
  separate git repository listing the repositories and holding tasks and tech lead artifacts.
  The task folder format is the same.
- **Trackers in the first version:** GitHub Issues, Jira, Linear. Git hosting and CI: GitHub with
  Actions, GitLab with CI. Observability: a generic capability "run a query against metrics or
  logs", configured by the project as a command or an MCP server.
- Integrations are bound by capability: "tracker" can be Jira or Linear. Nothing is connected
  silently.

## 12. Tech lead loop

Built after the developer loop (§20). Outline:

- **Rhythm:** on demand, plus scheduled runs through the project's CI. Vendor schedulers are
  optional.
- **Artifacts** in the team workspace:
  - shared rules for people and agents (`AGENTS.md`);
  - finished-work log, generated: id, start, end, executor kind, class, size, cost, review wait,
    outcome, link;
  - commitments and risks register: each commitment with forecast, evidence level, assumptions,
    early-warning triggers, owner, escalation rule, review date;
  - operations policy: autonomy ladder in incidents, rollback thresholds, dependency update
    rules, release rules, postmortem template;
  - agent registry: permissions, human owner, budget, work-in-progress limit, review rules per
    class;
  - onboarding for people and agents;
  - generated forecasts and weekly reports.
- **Never in the repository:** 1:1 notes and evaluations of people.
- **Forecasts:** the CLI runs a Monte Carlo simulation over the history of similar tasks and
  returns a range with probabilities. The agent collects risk signals. A human decides whether to
  name a date to anyone.
- **Statuses come from artifacts:** the status of a task is what its evidence and transitions
  say, not what someone reported.
- **Triage:** the agent proposes duplicates, links, missing information and a draft risk class
  for incoming items. Priority is the tech lead's decision; the agent does not rank by value.
- **Risk signals** are produced by rules the CLI evaluates: work older than its expected time,
  no activity for N days, stale evidence, a forecast range that moved. The agent may add signals
  it sees; those are marked as its reading.
- **Reports** show generated numbers and tables. The interpretation is written by the tech lead,
  or drafted by the agent and marked as a draft.
- **Notifications** are a plug-in module. By default it sends only "waiting for a human" and
  "blocked", as `{task, from, to, waiting_for, reason}`.

## 13. Memory and lessons

- Durable knowledge lives only in repository files: decisions, glossary, rules, lessons.
  leadyard does not use or write the agents' built-in memory.
- **Lesson format:** problem, root cause, fix, prevention, tags, files, and two required fields:
  where it was promoted (a rule, a check, a zone, a checklist item) and a review date. The root
  cause written by the agent is marked as its conclusion until a human promotes the lesson. A rule
  is created from a lesson only with a human's approval.

## 14. Third-party skills

- Default pack: mattpocock/skills. Other packs (and any skill) can be configured per stage.
- Every third-party skill and hook is pinned by source, version and content hash in
  `.leadyard/lock.yaml`. `leadyard lock verify` runs in CI.
- An update of a pack arrives as a normal PR where the skill diff is visible. Nothing changes
  silently. There is no content scanner in the first version.
- Attribution: each vendored or referenced pack keeps its license and is credited in `NOTICE`
  and in the docs. Packs without a license file are referenced, not copied.

## 15. Answer style

A project setting with a default: short answers, "before → after" with an example for each
change, no internal labels or jargon, one changelog line.

## 16. Telemetry

Only a local event log (`events.jsonl` in the task folder). Nothing is sent anywhere. The
tech lead reports are built by the CLI from these logs. Metrics the framework needs to judge
itself: false "done" rate (verdict returned), "checks green but human rejected", rework rounds,
time waiting for a human.

## 17. Testing leadyard itself

1. **Static, on every PR:** schemas, skill format validation, links and paths inside skills,
   freshness of generated adapter files (regenerate and compare).
2. **Deterministic, without a model:** install is idempotent; update keeps local edits;
   migrations from every previous schema; config layer merging; staleness; transitions;
   negative controls (§5.4.7).
3. **Live skill runs** on every certified agent: each scenario 3+ times, with a baseline
   "without the skill", judged with the method of §5.6. A single run is executed by a
   runner behind an interface; the first implementation is
   [ai-tester](https://github.com/lee-to/ai-tester), run in a container with a pinned version.
   Repetition, baseline and verdict are leadyard's.
4. **Rule:** a change to a skill is not accepted without a before/after measurement.
5. Fast track on every PR; full live matrix weekly and when a new model is released.

## 18. Repository layout and packaging

```
leadyard/
  README.md  AGENTS.md  CLAUDE.md  LICENSE  NOTICE  CONTRIBUTING.md  SECURITY.md
  spec/                 public API: JSON Schemas, stages, transitions, record formats
  skills/<name>/        own reference skills (portable front matter only)
  hooks/                hook entry points (thin; call the CLI)
  adapters/<agent>/     declarative adapter manifests and templates
  cmd/leadyard/         CLI (Go)
  internal/             CLI packages
  tests/{static,deterministic,live}/
  docs/
```

- **Install:** `leadyard init` asks which agents, writes the config and generates adapter files.
  A manifest (`.leadyard/manifest.json`) records every installed file with its SHA-256.
- **Update:** touches only files whose hash still matches; files the user changed are kept and
  reported.
- **Overrides:** a project overrides a reference skill by placing its version under
  `.leadyard/overrides/`; updates never touch overrides.
- **`leadyard migrate`** exists from the first release.
- **Versioning:** semver; the public API is `spec/`, stage and artifact identifiers, skill
  contracts, CLI flags. Before 1.0 the `0.y.z` rule applies and the README says so.
- **License:** MIT.

## 19. CLI surface (sketch)

| Command | Does |
|---|---|
| `leadyard init` | install for chosen agents |
| `leadyard doctor` | config, versions, probes; prints declared / enabled / reason |
| `leadyard task new\|show\|list` | task folders |
| `leadyard transition <event>` | the only way to change status |
| `leadyard decide` | record a human decision |
| `leadyard evidence add\|list` | record and list evidence |
| `leadyard level` | current level, stale records, what is missing for the class |
| `leadyard gate` | hook entry point; allow or deny with a reason |
| `leadyard resume` | print task state for a new or compacted session |
| `leadyard zones check` | zones touched by the diff and the resulting class part |
| `leadyard compare` | noise baseline and comparison (§5.6) |
| `leadyard pr body` | build the draft PR body from the task |
| `leadyard config get\|set` | read and write config |
| `leadyard lock verify` | check pinned third-party files |
| `leadyard update\|migrate` | update and migrate installations |
| `leadyard explain <code>` | explain a diagnostic code |

All commands support `--json` and never prompt in `unattended` mode.

## 20. Build order

1. **Core and developer loop:** spec, CLI, own stage skills, adapters with probes for Claude Code,
   Codex and Cursor. leadyard is developed with leadyard from the first working commit.
2. **Tech lead loop.**
3. **Tracker integrations and notifications.**

"Ideal" is the quality bar for each step, not a requirement to ship everything at once.

## 21. Open questions

1. Verify the stage mapping of §6.3 against the pinned version of mattpocock/skills, including
   how its `implement` commits relative to §7.
2. Statistical method for §5.6 (sign test, bootstrap, or another) and the default tolerance.
3. Starting thresholds that need calibration on real data: PR size warning, watch window.
4. Final name. Renaming touches the CLI name, the config directory and the trailer namespace.
5. Whether a review verdict without open blocking findings may count as level 3 on its own, or
   only together with a mechanical check (§23).
6. The human time budget per task and per week, against the cost estimate in §23.

## 22. Prior art and credits

leadyard takes ideas, not code, from these public projects:

- [mattpocock/skills](https://github.com/mattpocock/skills): default stage skills.
- [lee-to/hlv](https://github.com/lee-to/hlv): file hash maps for evidence staleness, snapshot
  before and after a run, the list of evidence states.
- [lee-to/aif-handoff](https://github.com/lee-to/aif-handoff): attempt fencing, stable finding
  ids and convergence rules for review, structural error classes, branch drift checks.
- [lee-to/ai-factory](https://github.com/lee-to/ai-factory): verdict block as data, one writer per
  artifact, config written only by a script.
- [lee-to/ai-tester](https://github.com/lee-to/ai-tester): live skill runs in a sandbox.

## 23. Task shape

What the design hands to an agent, which judgments hide inside each task, and who makes them.
Columns: selection (what is worth doing or showing), acceptance (is it done and correct),
inference (what a result means), stop (how much more work). "was → now" shows the change made
in this revision. Hooks (why an agent at all): reading volume, exhaustive check, code, routine
(extract or draft from a named source that a mechanism or human checks cheaply).

Derived from the list of stages, not from recorded episodes. One probe was run (below).

| Task | Hook | Selection | Acceptance | Inference | Stop | Decision | Accepted by | Human cost |
|---|---|---|---|---|---|---|---|---|
| intake | reading volume | agent, scope fixed by contract (was: silent) | mechanism: required fields present | "already done?" as raw links; agent reading marked (was: agent) | CLI budget (was: silent) | reshape + split | CLI; human confirms class | 0.5 min |
| risk answers | routine | — | human confirms | agent, each answer quotes its source; zones by CLI | — | split | human | in the line above |
| clarify | routine | agent picks questions; assumptions listed (was: hidden) | human answers | recommendation marked | frontier empty | wrap | human | class 2–3: 3–5 min |
| plan | code | agent; class 3 human approves; out-of-scope list added | human (class 3) | sources per rule | — | wrap | human | class 3: 5 min |
| implement | code | — | evidence only; checkbox is progress (was: checkbox) | — | CLI budget + tool question (was: retries only) | reshape | CLI levels | — |
| red-green check | code | — | CLI: failed at A, passed at B, test unchanged (was: agent says) | — | — | reshape | CLI | — |
| scope audit, model part | exhaustive check | — | human: remove / keep | fresh session (was: implementer) | — | split | human | 1–2 min |
| review | exhaustive check | severity by agent | open question §21.5 | findings raw, ids by CLI (was: model ids) | 5 rounds, convergence rule | wrap | human at verdict | in verdict |
| verdict | — | — | human | not-verified list by CLI | — | mechanism | human | 2–4 min |
| stand | code | scenarios by agent, exclusions listed, expectations hashed before run (was: silent) | runner (was: silent) | §5.6 | config | reshape + split | runner; class 3 human approves list with plan | in plan approval |
| merge package | routine | — | human reads PR | verified lines from records only (was: free text) | — | reshape | human (already does) | — |
| watch | routine | queries from config | §5.6 comparison (was: agent) | agent summary marked | window from config (was: silent) | reshape | CLI | 0.5 min (report deploy) |
| triage (tech lead) | reading volume | priority by tech lead (was: silent) | tech lead | duplicates and gaps proposed | — | split | tech lead | tech lead's existing work |
| risk signals | exhaustive check | rules by CLI; agent extras marked | tech lead | — | — | split | CLI + tech lead | — |
| reports | routine | — | tech lead | interpretation by tech lead or marked draft (was: silent) | — | split | tech lead | weekly, ~10 min |
| lesson | routine | — | human on promotion | root cause marked as agent conclusion | — | wrap | human | per promotion |

Tests that fired (numbering of the agent-task shape check): self-declared "done" (1) for
implement, stand, watch, red-green; raw results mixed with conclusions (2) for intake, watch,
reports; silent scope narrowing (9) for intake, plan, stand; no external budget (8) for intake,
implement; no data acquisition step (12) for stand; value judgment (5) for triage; a skeptic
treated as acceptance (4) for review.

**Probe.** A reviewer in a fresh session (one run, a mid-tier model) received a request
("`date_to` inclusive"), a plan marked done and a diff with `created_at < date_to`. It found the
bug and the missing boundary test, returned `fail`, and numbered findings `F1`–`F5`. The tests the
implementer wrote passed with the bug present. One catch does not show how often a review
returns a false `pass`; that remains unmeasured.

**Human cost per task** (estimate, not measured): class 1 about 3.5 min (confirm class, scope
audit answers, verdict); class 2 about 10 min (plus clarify and scope approval); class 3 about
17 min (plus plan and scenario approval, deploy report). Merge and deploy are human work that
existed before. At a weekly volume of `n1`, `n2`, `n3` tasks per class:
`3.5·n1 + 10·n2 + 17·n3` minutes.
