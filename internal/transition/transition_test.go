package transition

import (
	"errors"
	"testing"

	"github.com/zireaelq/leadyard/internal/ledger"
	"github.com/zireaelq/leadyard/internal/task"
)

func newTask(t *testing.T, class int) *task.Task {
	t.Helper()
	tk, err := task.New(t.TempDir(), "T-1", "x", "feature", "", "req", "")
	if err != nil {
		t.Fatal(err)
	}
	tk.Meta.Class = class
	return tk
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestTableStatusesValid(t *testing.T) {
	rows, err := Table()
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]bool{}
	for _, s := range task.Statuses {
		valid[s] = true
	}
	for _, r := range rows {
		if !valid[r.To] {
			t.Errorf("%s: unknown target %q", r.Event, r.To)
		}
		for _, f := range r.From {
			if !valid[f] {
				t.Errorf("%s: unknown source %q", r.Event, f)
			}
		}
	}
}

func TestFlowAndRefusals(t *testing.T) {
	tk := newTask(t, 0)
	if err := Apply(tk, nil, Input{Event: "verify", By: "agent"}); code(err) != "not_allowed_from" {
		t.Fatalf("want not_allowed_from, got %v", err)
	}
	if err := Apply(tk, nil, Input{Event: "start", By: "agent"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(tk, nil, Input{Event: "implement", By: "agent"}); code(err) != "class_unconfirmed" {
		t.Fatalf("want class_unconfirmed, got %v", err)
	}
	tk.Meta.Class = 2
	if err := Apply(tk, nil, Input{Event: "implement", By: "agent"}); code(err) != "scope_unapproved" {
		t.Fatalf("want scope_unapproved, got %v", err)
	}
	recs := []ledger.Record{{Type: ledger.TypeDecision, Kind: "scope_approved", Attempt: 1}}
	if err := Apply(tk, recs, Input{Event: "implement", By: "agent"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(tk, recs, Input{Event: "verify", By: "agent"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(tk, recs, Input{Event: "approve_verdict", By: "agent", Level: 3, Req: 3}); code(err) != "actor_mismatch" {
		t.Fatalf("want actor_mismatch, got %v", err)
	}
	if err := Apply(tk, recs, Input{Event: "approve_verdict", By: "human:a", Level: 2, Req: 3}); code(err) != "evidence_missing" {
		t.Fatalf("want evidence_missing, got %v", err)
	}
	if err := Apply(tk, recs, Input{Event: "request_changes", By: "human:a"}); code(err) != "reason_required" {
		t.Fatalf("want reason_required, got %v", err)
	}
	if err := Apply(tk, recs, Input{Event: "request_changes", By: "human:a", Reason: "boundary"}); err != nil {
		t.Fatal(err)
	}
	if tk.Meta.Attempt != 2 || tk.Meta.Status != "in_progress" {
		t.Fatalf("meta after request_changes: %+v", tk.Meta)
	}
	all, _ := ledger.Read(tk.Dir)
	if len(all) != 4 {
		t.Fatalf("expected 4 decision records, got %d", len(all))
	}
}
