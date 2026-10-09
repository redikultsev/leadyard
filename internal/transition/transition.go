// Package transition applies events from the transition table to a task
// (design §5.3) and records them as decisions.
package transition

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/redikultsev/leadyard/internal/ledger"
	"github.com/redikultsev/leadyard/internal/task"
	"github.com/redikultsev/leadyard/spec"
)

// Requires lists the conditions of a transition.
type Requires struct {
	Reason                bool `yaml:"reason"`
	ClassConfirmed        bool `yaml:"class_confirmed"`
	ScopeApprovedForClass int  `yaml:"scope_approved_for_class"`
	PlanApprovedForClass  int  `yaml:"plan_approved_for_class"`
	LevelMeetsClass       bool `yaml:"level_meets_class"`
}

// Row is one transition.
type Row struct {
	Event    string   `yaml:"event"`
	From     []string `yaml:"from"`
	Actor    string   `yaml:"actor"`
	To       string   `yaml:"to"`
	Requires Requires `yaml:"requires"`
	Effects  []string `yaml:"effects"`
}

type table struct {
	Transitions []Row `yaml:"transitions"`
}

// Table returns the transition table.
func Table() ([]Row, error) {
	var t table
	if err := yaml.Unmarshal(spec.Transitions, &t); err != nil {
		return nil, err
	}
	return t.Transitions, nil
}

// Error is a refusal with a structured code.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// Input carries what the caller knows about the event.
type Input struct {
	Event  string
	By     string // "agent" or "human:<name>"
	Reason string
	Level  int // current evidence level, for level_meets_class
	Req    int // required level for the class
}

// Apply checks and applies an event to t, appends a decision record to the
// ledger, and writes the decision line into task.md.
func Apply(t *task.Task, recs []ledger.Record, in Input) error {
	rows, err := Table()
	if err != nil {
		return err
	}
	var row *Row
	for i := range rows {
		if rows[i].Event == in.Event {
			row = &rows[i]
		}
	}
	if row == nil {
		return &Error{"unknown_event", fmt.Sprintf("%q; known: %s", in.Event, names(rows))}
	}
	if !contains(row.From, t.Meta.Status) {
		return &Error{"not_allowed_from", fmt.Sprintf("%s is not allowed from %s (allowed from %s)", in.Event, t.Meta.Status, strings.Join(row.From, ", "))}
	}
	isHuman := strings.HasPrefix(in.By, "human")
	if row.Actor == "human" && !isHuman {
		return &Error{"actor_mismatch", fmt.Sprintf("%s is a human decision; pass --by human:<name>", in.Event)}
	}
	r := row.Requires
	if r.Reason && strings.TrimSpace(in.Reason) == "" {
		return &Error{"reason_required", in.Event + " needs --reason"}
	}
	if r.ClassConfirmed && t.Meta.Class == 0 {
		return &Error{"class_unconfirmed", "a human must confirm the risk class first: leadyard decide class --value N --by human:<name>"}
	}
	if r.ScopeApprovedForClass > 0 && t.Meta.Class >= r.ScopeApprovedForClass && !hasDecision(recs, t.Meta.Attempt, "scope_approved") {
		return &Error{"scope_unapproved", fmt.Sprintf("class %d needs a recorded scope approval", t.Meta.Class)}
	}
	if r.PlanApprovedForClass > 0 && t.Meta.Class >= r.PlanApprovedForClass && !hasDecision(recs, t.Meta.Attempt, "plan_approved") {
		return &Error{"plan_unapproved", fmt.Sprintf("class %d needs a recorded plan approval", t.Meta.Class)}
	}
	if r.LevelMeetsClass && (in.Req == 0 || in.Level < in.Req) {
		return &Error{"evidence_missing", fmt.Sprintf("level %d is below the required %d; use accept_with_risk with a reason to accept anyway", in.Level, in.Req)}
	}

	from := t.Meta.Status
	t.Meta.Status = row.To
	for _, e := range row.Effects {
		if e == "new_attempt" {
			t.Meta.Attempt++
		}
	}
	rec, err := ledger.Append(t.Dir, ledger.Record{Type: ledger.TypeDecision, Task: t.Meta.ID, Attempt: t.Meta.Attempt,
		Kind: "transition", Ref: in.Event, Value: from + " -> " + row.To, By: in.By, Reason: in.Reason})
	if err != nil {
		return err
	}
	t.AppendToSection("Decisions", DecisionLine(rec))
	return t.Save()
}

// DecisionLine renders a decision for task.md.
func DecisionLine(r ledger.Record) string {
	s := fmt.Sprintf("- %s · %s · %s", r.RecordedAt.Format(time.DateTime), r.By, r.Kind)
	if r.Ref != "" {
		s += " " + r.Ref
	}
	if r.Value != "" {
		s += " = " + r.Value
	}
	if r.Reason != "" {
		s += " · \"" + r.Reason + "\""
	}
	return s
}

func hasDecision(recs []ledger.Record, attempt int, kind string) bool {
	for _, r := range recs {
		if r.Type == ledger.TypeDecision && r.Kind == kind && r.Attempt == attempt {
			return true
		}
	}
	return false
}

func names(rows []Row) string {
	var n []string
	for _, r := range rows {
		n = append(n, r.Event)
	}
	return strings.Join(n, ", ")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
