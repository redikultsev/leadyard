---
name: leadyard-clarify
description: leadyard clarify stage. Ask the human every blocking question in one round, with options and a recommendation, and record the answers and the scope approval. Use when a leadyard task has status "clarifying".
license: MIT
---

# Clarify

1. Ask only what blocks the next step. Put every question whose prerequisites are settled into
   ONE message. For each: the options, your recommendation, and why.
2. In the same message list the assumptions you made without asking.
3. Ask the human to confirm the risk class if it is not confirmed yet.
4. Record each answer as a decision, quoting the human:
   - class: `leadyard decide class --value N --by human:<name> --reason "<their words>"`
   - scope (class 2+): `leadyard decide scope_approved --by human:<name> --reason "<their words>"`
   - other answers: `leadyard decide answer --ref "<question>" --value "<answer>" --by human:<name>`
5. Class 3: `leadyard transition plan --by agent`. Otherwise `leadyard transition implement --by agent`.
