// Package review builds the input for a fresh-session reviewer and records its
// verdict block (design §5.8.1).
package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zireaelq/leadyard/internal/ledger"
	"github.com/zireaelq/leadyard/internal/repo"
	"github.com/zireaelq/leadyard/internal/task"
)

// Verdict is the JSON inside a ```leadyard-verdict block.
type Verdict struct {
	Schema   int              `json:"schema"`
	Stage    string           `json:"stage"`
	Status   string           `json:"status"`
	Findings []ledger.Finding `json:"findings"`
	Previous []PreviousState  `json:"previous"`
}

// PreviousState is the reviewer's report on a finding from an earlier round.
type PreviousState struct {
	ID    string `json:"id"`
	State string `json:"state"` // fixed, open, replaced
}

var blockRe = regexp.MustCompile("(?s)```leadyard-verdict[ \\t]*\\n(.*?)\\n```")

// Parse extracts and validates the last verdict block in text.
func Parse(text string) (Verdict, error) {
	var v Verdict
	m := blockRe.FindAllStringSubmatch(text, -1)
	if len(m) == 0 {
		return v, errors.New("no ```leadyard-verdict block found; a missing block counts as fail")
	}
	if err := json.Unmarshal([]byte(m[len(m)-1][1]), &v); err != nil {
		return v, fmt.Errorf("verdict block is not valid JSON: %w", err)
	}
	if v.Schema != 1 {
		return v, fmt.Errorf("verdict schema %d is not supported (expected 1)", v.Schema)
	}
	switch v.Status {
	case "pass", "warn", "fail":
	default:
		return v, fmt.Errorf("verdict status %q must be pass, warn or fail", v.Status)
	}
	for i, f := range v.Findings {
		if f.Severity != "blocking" && f.Severity != "advisory" {
			return v, fmt.Errorf("finding %d: severity %q must be blocking or advisory", i+1, f.Severity)
		}
		if strings.TrimSpace(f.Summary) == "" {
			return v, fmt.Errorf("finding %d: empty summary", i+1)
		}
	}
	for _, p := range v.Previous {
		switch p.State {
		case "fixed", "open", "replaced":
		default:
			return v, fmt.Errorf("previous finding %s: state %q must be fixed, open or replaced", p.ID, p.State)
		}
	}
	return v, nil
}

var spaceRe = regexp.MustCompile(`\s+`)

// FindingID is computed by the CLI: ids written by a model are not stable.
func FindingID(f ledger.Finding) string {
	norm := strings.ToLower(spaceRe.ReplaceAllString(strings.TrimSpace(f.File+" "+f.Summary), " "))
	sum := sha256.Sum256([]byte(norm))
	return "f-" + hex.EncodeToString(sum[:3])
}

// OpenFindings returns the findings still open after the latest review of the
// current attempt.
func OpenFindings(recs []ledger.Record, attempt int) []ledger.Finding {
	var last *ledger.Record
	for i := range recs {
		if recs[i].Type == ledger.TypeReview && recs[i].Attempt == attempt {
			last = &recs[i]
		}
	}
	if last == nil {
		return nil
	}
	return last.Findings
}

// Record turns a verdict into a review record. Previous findings stay open
// unless the reviewer reports them fixed or replaced; a finding the reviewer
// repeats with a known id keeps that id.
func Record(r *repo.Repo, t *task.Task, recs []ledger.Record, v Verdict) (ledger.Record, error) {
	prev := OpenFindings(recs, t.Meta.Attempt)
	closed := map[string]bool{}
	for _, p := range v.Previous {
		if p.State != "open" {
			closed[p.ID] = true
		}
	}
	known := map[string]bool{}
	var open []ledger.Finding
	for _, f := range prev {
		if !closed[f.ID] {
			open = append(open, f)
			known[f.ID] = true
		}
	}
	for _, f := range v.Findings {
		if !known[f.ID] {
			f.ID = FindingID(f)
		}
		if known[f.ID] {
			continue
		}
		known[f.ID] = true
		open = append(open, f)
	}
	changed, err := r.ChangedFiles(t.Meta.Base)
	if err != nil {
		return ledger.Record{}, err
	}
	cov, err := r.HashFiles(changed)
	if err != nil {
		return ledger.Record{}, err
	}
	return ledger.Append(t.Dir, ledger.Record{Type: ledger.TypeReview, Task: t.Meta.ID, Attempt: t.Meta.Attempt,
		Status: v.Status, Findings: open, Covers: cov})
}

// Input builds the reviewer's bundle: request, criteria, plan, open findings
// with their ids, and the diff against the task's base including new files.
// The implementing session does not write the reviewer's prompt.
func Input(r *repo.Repo, t *task.Task, recs []ledger.Record) (string, error) {
	var b strings.Builder
	b.WriteString("# Review input for task " + t.Meta.ID + " (attempt " + fmt.Sprint(t.Meta.Attempt) + ")\n\n")
	b.WriteString("You are a reviewer in a fresh session. Review only what is below. End your answer with\n")
	b.WriteString("one ```leadyard-verdict block: {\"schema\":1,\"stage\":\"review\",\"status\":\"pass|warn|fail\",\n")
	b.WriteString("\"findings\":[{\"severity\":\"blocking|advisory\",\"file\":\"...\",\"line\":0,\"summary\":\"...\"}],\n")
	b.WriteString("\"previous\":[{\"id\":\"...\",\"state\":\"fixed|open|replaced\"}]}. Report the state of every\n")
	b.WriteString("previous finding by its id. Also flag additions the request did not ask for (flags, env vars,\n")
	b.WriteString("files, hardcoded values) as findings.\n\n")
	b.WriteString("## Request (verbatim)\n\n" + t.Section("Request (verbatim)") + "\n\n")
	if c := t.Section("Acceptance criteria"); c != "" {
		b.WriteString("## Acceptance criteria\n\n" + c + "\n\n")
	}
	if p := t.Section("Plan"); p != "" {
		b.WriteString("## Plan\n\n" + p + "\n\n")
	}
	if open := OpenFindings(recs, t.Meta.Attempt); len(open) > 0 {
		b.WriteString("## Previous findings\n\n")
		for _, f := range open {
			b.WriteString(fmt.Sprintf("- %s [%s] %s:%d %s\n", f.ID, f.Severity, f.File, f.Line, f.Summary))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Diff\n\n```diff\n")
	args := []string{"diff"}
	if t.Meta.Base != "" {
		args = append(args, t.Meta.Base)
	}
	args = append(args, "--", ".", ":(exclude).leadyard")
	out, err := r.Git(args...)
	if err != nil {
		return "", err
	}
	b.WriteString(out)
	untracked, err := r.Git("ls-files", "--others", "--exclude-standard", "--", ".", ":(exclude).leadyard")
	if err != nil {
		return "", err
	}
	for _, f := range strings.Fields(untracked) {
		content, err := os.ReadFile(filepath.Join(r.Root, f))
		if err != nil {
			continue
		}
		b.WriteString("--- /dev/null\n+++ b/" + f + " (new file)\n")
		for _, l := range strings.Split(strings.TrimRight(string(content), "\n"), "\n") {
			b.WriteString("+" + l + "\n")
		}
	}
	b.WriteString("```\n")
	return b.String(), nil
}
