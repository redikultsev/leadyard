---
name: leadyard-plan
description: leadyard plan stage for class 3 tasks. Write a numbered checkbox plan with sources and an out-of-scope list, and get it approved. Use when a leadyard task has status "planning".
license: MIT
---

# Plan

1. Write `## Plan` in task.md:
   - numbered steps as `- [ ] 1. ...`, each naming the files it touches in backticks;
   - every significant rule cites its source: the request, a decision, or the code (`file:line`);
   - a final `- Out of scope: ...` line.
2. If two sources conflict and no decision resolves it, stop: `leadyard transition ask_human --by agent --reason "..."`.
3. Show the plan to the human. When they approve:
   `leadyard decide plan_approved --by human:<name> --reason "<their words>"`, then
   `leadyard transition implement --by agent`.
