// Package render produces the text people and agents read: the resume block
// after compaction, the level report and the draft PR body.
package render

import (
	"fmt"
	"strings"

	"github.com/redikultsev/leadyard/internal/ledger"
	"github.com/redikultsev/leadyard/internal/level"
	"github.com/redikultsev/leadyard/internal/task"
	"github.com/redikultsev/leadyard/internal/transition"
)

// MaxResume keeps the resume block under the agent's hook output cap
// (Claude Code shows only a preview above 10,000 characters).
const MaxResume = 9000

// next suggests the next step for a status.
var next = map[string]string{
	"new":            "intake: fill Request, Acceptance criteria; propose a class (leadyard decide proposed_class); then `leadyard transition start`",
	"clarifying":     "ask the blocking questions in one round; record answers (leadyard decide); class 2+: get the scope approved",
	"planning":       "write the Plan with an out-of-scope list; class 3: get the plan approved",
	"in_progress":    "implement the plan; run checks with `leadyard run <check>`; then `leadyard transition verify`",
	"verifying":      "get a fresh-session review (`leadyard review input` → reviewer → `leadyard review record`), run `leadyard scope`, then show `leadyard level` to the human",
	"waiting_human":  "wait for the human's decision; do not continue on your own",
	"ready_to_merge": "build the PR body (`leadyard pr-body`); the human merges",
	"merged":         "the human reports the deploy (`leadyard transition deployed`) or closes",
	"deployed":       "close when the human confirms",
	"on_hold":        "on hold; do nothing until the human resumes it",
}

// Resume renders the task state for a new or compacted session.
func Resume(t *task.Task, rep level.Report, recs []ledger.Record, delivery string) string {
	m := t.Meta
	var b strings.Builder
	fmt.Fprintf(&b, "leadyard task %s — %s\n", m.ID, m.Title)
	fmt.Fprintf(&b, "status: %s · kind: %s · attempt: %d · delivery: %s\n", m.Status, m.Kind, m.Attempt, delivery)
	if m.Class == 0 {
		fmt.Fprintf(&b, "class: not confirmed (proposed %d)\n", m.ProposedClass)
	} else {
		fmt.Fprintf(&b, "class: %d · level %d of required %d\n", m.Class, rep.Level, rep.Required)
	}
	if n, ok := next[m.Status]; ok {
		b.WriteString("next: " + n + "\n")
	}
	if pl := t.Plan(); len(pl) > 0 {
		b.WriteString("\nplan:\n")
		for _, it := range pl {
			mark := " "
			if it.Done {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, it.Text)
		}
	}
	if len(rep.NotVerified) > 0 {
		b.WriteString("\nnot verified:\n")
		for _, s := range rep.NotVerified {
			b.WriteString("- " + s + "\n")
		}
	}
	var decisions []string
	for _, r := range recs {
		if r.Type == ledger.TypeDecision {
			decisions = append(decisions, transition.DecisionLine(r))
		}
	}
	if len(decisions) > 5 {
		decisions = decisions[len(decisions)-5:]
	}
	if len(decisions) > 0 {
		b.WriteString("\nlast decisions:\n" + strings.Join(decisions, "\n") + "\n")
	}
	if notes := t.Section("Notes"); notes != "" {
		b.WriteString("\nnotes:\n" + notes + "\n")
	}
	b.WriteString("\nfull file: " + t.Dir + "/task.md\n")
	s := b.String()
	if len(s) > MaxResume {
		s = s[:MaxResume-60] + "\n… truncated; read the full task.md\n"
	}
	return s
}

// Level renders a level report for the human at the verdict.
func Level(rep level.Report) string {
	var b strings.Builder
	if rep.Required == 0 {
		fmt.Fprintf(&b, "level %d · class not confirmed\n", rep.Level)
	} else {
		state := "meets the requirement"
		if rep.Below {
			state = "BELOW REQUIREMENT"
		}
		fmt.Fprintf(&b, "level %d of required %d (class %d) · %s\n", rep.Level, rep.Required, rep.Class, state)
	}
	b.WriteString("\nchecks:\n")
	for _, c := range rep.Checks {
		line := fmt.Sprintf("- %s (level %d): %s", c.ID, c.Level, c.Status)
		if c.Artifact != "" {
			line += " · " + c.Artifact
		}
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "- review: %s\n", rep.ReviewStatus)
	if len(rep.Criteria) > 0 {
		b.WriteString("\nacceptance criteria:\n")
		for _, c := range rep.Criteria {
			st := "not verified"
			if c.Verified {
				st = "verified by " + strings.Join(c.Checks, ", ")
			}
			fmt.Fprintf(&b, "- %s %s: %s\n", c.ID, c.Text, st)
		}
	}
	if len(rep.OpenBlocking) > 0 {
		b.WriteString("\nopen blocking findings:\n")
		for _, f := range rep.OpenBlocking {
			fmt.Fprintf(&b, "- %s %s:%d %s\n", f.ID, f.File, f.Line, f.Summary)
		}
	}
	if len(rep.OpenAdvisory) > 0 {
		b.WriteString("\nopen advisory findings:\n")
		for _, f := range rep.OpenAdvisory {
			fmt.Fprintf(&b, "- %s %s:%d %s\n", f.ID, f.File, f.Line, f.Summary)
		}
	}
	if len(rep.UnacceptedEdit) > 0 {
		b.WriteString("\npre-existing tests edited (accept each: leadyard decide test_edit_accepted --ref <path> --by human:<name>):\n")
		for _, f := range rep.UnacceptedEdit {
			b.WriteString("- " + f + "\n")
		}
	}
	if len(rep.NotVerified) > 0 {
		b.WriteString("\nnot verified:\n")
		for _, s := range rep.NotVerified {
			b.WriteString("- " + s + "\n")
		}
	}
	return b.String()
}

// PRBody renders the draft pull request description from the task.
func PRBody(t *task.Task, rep level.Report) string {
	m := t.Meta
	var b strings.Builder
	b.WriteString("## " + m.Title + "\n\n")
	if m.Tracker != "" {
		b.WriteString("Closes " + m.Tracker + "\n\n")
	}
	b.WriteString("### Request\n\n" + quote(t.Section("Request (verbatim)")) + "\n\n")
	if pl := t.Plan(); len(pl) > 0 {
		b.WriteString("### What changed\n\n")
		for _, it := range pl {
			if it.Done {
				b.WriteString("- " + it.Text + "\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("### How it was verified\n\n")
	b.WriteString(Level(rep))
	if rn := t.Section("Release notes"); rn != "" {
		b.WriteString("\n### Deploy and QA notes (written by the agent)\n\n" + rn + "\n")
	}
	b.WriteString(fmt.Sprintf("\n<sub>leadyard task %s, attempt %d. Verified lines come from evidence records; levels are tamper-evident, not tamper-proof.</sub>\n", m.ID, m.Attempt))
	return b.String()
}

func quote(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}
