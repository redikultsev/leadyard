---
name: leadyard-intake
description: leadyard intake stage. Collect facts for a new task, quote the request and acceptance criteria into task.md, and propose a risk class. Use when a leadyard task has status "new".
license: MIT
---

# Intake

Goal: the task file holds the request verbatim, the acceptance criteria, the facts, and a
proposed risk class. You do not change code in this stage.

1. Copy the request into `## Request (verbatim)` exactly as given (ticket text, user message).
2. Write `## Acceptance criteria` as `- C1. ...` lines, quoted from the ticket. If the ticket has
   none, derive them from the request and mark each "(derived)". Do not invent requirements.
3. Search, and list in `## Notes` what you searched and what you skipped:
   - the tracker item and linked items, if you can read them;
   - code for the terms of the request;
   - branches and recent commits for prior work ("is it already done?"). Answer with links
     (branch, commit). Mark any conclusion as "my reading".
4. Propose the risk class. Run `leadyard zones` for zone hits. Answer the five questions, each
   with the line of the request or code it rests on:
   1. Is the change reversible?
   2. Does it touch customer money or data?
   3. Does it change model behavior or another non-deterministic part?
   4. Does it change a contract with another system?
   5. Does it touch security?
   The class is the highest of zones and answers; an unanswered question raises it.
   Record: `leadyard decide proposed_class --value N --by agent --reason "..."`.
5. `leadyard transition start --by agent`. Show the human the criteria, the class proposal with
   reasons, and ask them to confirm the class.
