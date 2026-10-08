# Decisions

Each decision has a number, a date, the decision and the reason. Accepted decisions are not
edited; a new entry replaces an old one and links to it. Sections refer to
[design.md](design.md).

## 2026-10-08 — initial design

### Scope

1. **Open framework from day one,** for any developer and tech lead, not tied to any project or
   stack. Examples in docs are neutral.
2. **Several agents from the start.** No interim "one agent only" version.
3. **The tech lead loop is in scope in full:** intake and triage, statuses, commitments with
   confidence, review queue, post-deploy watch (§12). It is built after the developer loop (49).
4. **Separate repository,** usable next to any agent harness.

### Shape

5. **Three layers:** spec, reference skills and hooks, CLI (§4).
6. **Stages are contracts.** Each has a default skill and can use any other skill that satisfies
   the contract (§5.8, §6.3).
7. **Task location is configurable:** a folder in the repository by default, a team workspace
   for tasks across repositories. One format (§5.1, §11).
8. **Tracker holds what, who and status for the team; the task file holds evidence, decisions
   and agent state.** A sync adapter connects them; a tracker is optional (§5.1).
9. **Core on common formats plus adapters per agent,** each with a declared support level
   (§9).
10. **Everything that must be the same every time is computed by the CLI, not the model**
    (§3.4).
11. **CLI in Go.** One static binary; a hook calls it on every tool use without a runtime.
12. **Framework language is English;** the agent answers the user in the user's language.

### Evidence and risk

13. **"Done" is one evidence ladder in the core, levels 0–5.** A project only maps its checks to
    levels. A level counts only with an artifact link (§5.4).
14. **Evidence goes stale when covered files change;** unknown coverage goes stale on any change
    (§5.4).
15. **Level 3 is a check the implementing session cannot influence:** the existing test suite or
    a review in a fresh session without the implementation context. A different model is not
    required (§5.4).
16. **Three risk classes.** Defaults: class 1 → level 1; class 2 → level 3 and scope approval;
    class 3 → level 4 before merge and 5 before close, scope and plan approval (§5.5).
17. **Class comes from zones plus five fixed questions;** highest wins; a missing answer raises
    the class. The agent proposes, a human confirms. Reason: file paths alone miss changes such as
    a one-line prompt edit that alters paid outputs (§5.5).
18. **Noise-aware verdict in the core for every project:** noise baseline, comparison, four
    outcomes. The project supplies the runner, the CLI computes the statistics (§5.6).

### Process

19. **Developer loop stages and their binding to classes** as in §6.1. The scope audit runs in
    every class because unrequested additions are the most frequent human correction.
20. **Always human: accepting the verdict, merging into the main branch, deploying.** Other gates
    are switched on by class (§5.7).
21. **Gates are answered in chat and recorded in the task file:** who, what, when, reason (§5.7).
22. **Statuses are fixed in the core.** A project can hide statuses, not add them (§5.2).
23. **The transition table is in the core.** A project may tighten it; loosening is explicit and
    recorded (§5.3).
24. **Two skill modes:** interactive (may ask) and unattended (never asks; stops with
    `needs_human` and a reason) (§5.9).
25. **Limits default to 5** for review rounds and automatic retries; a human may raise them for
    one task, recorded (§5.8).

### Git

26. **Three lists:** on its own / with confirmation each time / never (§7.1). Default delivery
    mode `pr`.
27. **Branch names follow the team convention:** from config, or inferred from existing branches
    and confirmed once; fallback `agent/<task>` (§7.3).
28. **One task per working tree; parallel work only in separate worktrees created with
    confirmation** (§7.3).
29. **Identity:** bot account or human account with a trailer, both supported, bot recommended
    for teams. One trailer `Assisted-by: <agent>/<model>`; the agent's own co-author trailer is
    off. A switch forbids AI contributions (§7.4).

### Guardrails and agents

30. **Baseline mechanical denies are on by default.** Tighten freely; loosen only explicitly with
    a reason. Built-in probes check that denies actually fire (§8).
31. **Agent tiers:** certified Claude Code, Codex, Cursor; experimental Gemini CLI, Copilot CLI,
    OpenCode; others instructions only (§9.1).

### Configuration and integrations

32. **`.leadyard/config.yaml` with a schema, three layers:** distribution < team (in git) <
    personal (not in git). Nothing is enabled silently (§10).
33. **First integrations:** GitHub Issues, Jira, Linear; GitHub with Actions, GitLab with CI;
    observability as a generic "query metrics or logs" capability (§11).
34. **Team workspace is a separate git repository** (§11).
35. **Notifications are a plug-in module;** by default only "waiting for a human" and "blocked"
    (§12).

### Skills

36. **Default skill pack is mattpocock/skills;** stages it does not cover get leadyard's own
    skills; every skill is replaceable (§6.3).
37. **Third-party skills and hooks are pinned by version and hash;** updates arrive as a PR with
    a visible diff. No content scanner in the first version (§14).
38. **Live skill runs use ai-tester behind a runner interface;** repetition, baseline and verdict
    are leadyard's; runs happen in a container with a pinned version (§17).
39. **Evidence format is leadyard's own,** modelled on hlv, without an hlv adapter in the first
    version.

### Tech lead

40. **Tech lead rhythm:** on demand plus scheduled runs through the project's CI (§12).
41. **Tech lead artifacts** as listed in §12, in the team workspace. 1:1 notes and evaluations of
    people never go into a repository.
42. **Dates are forecast by the CLI from the history of similar tasks** (a range with
    probabilities). The agent collects risk signals; a human decides whether to name a date.

### Project

43. **License: MIT** for everything.
44. **Telemetry: a local event log only;** nothing is sent (§16).
45. **leadyard is tested with static checks, deterministic tests and live skill runs** (3+ runs,
    baseline without the skill); a skill change without a before/after measurement is not
    accepted (§17).
46. **Memory is only files in the repository;** agents' built-in memory is not used (§13).
47. **Answer style is a project setting** with a default (§15).
48. **No own control panel;** trackers, git hosting and the IDE show the work (§2).
49. **Build order:** core and developer loop with probes for the three certified agents, then the
    tech lead loop, then tracker integrations and notifications. "Ideal" is a quality bar, not a
    big-bang release (§20).
50. **Dogfooding:** leadyard is developed with leadyard from the first working commit.
51. **Working name `leadyard`;** the final name is chosen later.

## 2026-10-08 — after the task shape check (design §23)

52. **Level 3 requires a mechanical check** (replaces 15): the project's pre-existing tests pass
    and were not edited in the diff; a fresh-session review with no open blocking findings is a
    required condition but never raises the level by itself. A project without tests lowers the
    requirement explicitly with a recorded reason. Reason: a reviewer is a model too; it reduces
    defects but is not acceptance (§5.4).
53. **Human time budget:** about one hour a week for a single developer at six tasks; estimate
    3.5 / 10 / 17 minutes per task of class 1 / 2 / 3 (§23).

## 2026-10-08 — after the design review (design §24, §25)

54. **Files in the repository stay the source of truth in 0.1.** Records are tamper-evident, not
    tamper-proof; the agent records the human's gate answers. Moving authority to the git host is
    revisited only if forgery or branch state becomes a real problem (§25 row 29).
55. **Simple first.** A version ships the smallest set that runs end to end; complexity is added
    when a real case needs it (principle 9, §24).
56. **Version 0.1 runs the developer loop end to end on Claude Code;** Codex and Cursor follow in
    0.2 (§20, §24). Decision 31 stands.
57. **Delivery mode `none`:** the agent edits files only; the human does all git writes. A user-level
    config may only tighten git policy (§7.2, §10.1).
58. **Level is cumulative over the latest record of each check;** new tests do not affect level 3,
    edited pre-existing tests are accepted by the human (§5.4). Refines 52.
59. **Acceptance criteria are part of the task** and shown with their evidence at the verdict
    (§5.1, §5.7).
60. **0.1 uses own thin skills for every stage;** the mattpocock pack integration comes later
    because its skills do not fit the stage contracts as is (§6.3). Refines 36.
