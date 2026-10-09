package level

import (
	"testing"

	"github.com/redikultsev/leadyard/internal/config"
	"github.com/redikultsev/leadyard/internal/ledger"
	"github.com/redikultsev/leadyard/internal/repo"
	"github.com/redikultsev/leadyard/internal/task"
	"github.com/redikultsev/leadyard/internal/testutil"
)

type fixture struct {
	r   *repo.Repo
	cfg config.Config
	t   *task.Task
}

func setup(t *testing.T) fixture {
	t.Helper()
	r := testutil.NewRepo(t, map[string]string{"src/a.go": "a", "src/a_test.go": "t"})
	cfg := config.Default()
	cfg.Checks = []config.Check{
		{ID: "lint", Level: 1, Run: "true"},
		{ID: "tests", Level: 3, Run: "true", Tests: []string{"**/*_test.go"}},
	}
	tk, err := task.New(r.Root, "T-1", "x", "feature", "", "req", r.BaseCommit("main"))
	if err != nil {
		t.Fatal(err)
	}
	tk.Meta.Class = 2
	tk.AppendToSection("Acceptance criteria", "- C1. works")
	return fixture{r, cfg, tk}
}

func (f fixture) run(t *testing.T, id, status string, criteria ...string) {
	t.Helper()
	c, _ := f.cfg.Check(id)
	cov, err := Coverage(f.r, f.t.Meta.Base, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Append(f.t.Dir, ledger.Record{Type: ledger.TypeRun, Task: "T-1", Attempt: f.t.Meta.Attempt,
		Check: id, Level: c.Level, Status: status, Covers: cov, CheckDef: DefDigest(c), Criteria: criteria}); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) review(t *testing.T, findings ...ledger.Finding) {
	t.Helper()
	changed, _ := f.r.ChangedFiles(f.t.Meta.Base)
	cov, _ := f.r.HashFiles(changed)
	if _, err := ledger.Append(f.t.Dir, ledger.Record{Type: ledger.TypeReview, Task: "T-1", Attempt: f.t.Meta.Attempt,
		Covers: cov, Findings: findings}); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) compute(t *testing.T) Report {
	t.Helper()
	recs, err := ledger.Read(f.t.Dir)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Compute(f.r, f.cfg, f.t, recs)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestEmptyIsZero(t *testing.T) {
	f := setup(t)
	if rep := f.compute(t); rep.Level != 0 || !rep.Below || rep.Required != 3 {
		t.Fatalf("%+v", rep)
	}
}

func TestCumulativeAndConditions(t *testing.T) {
	f := setup(t)
	testutil.WriteFile(t, f.r.Root, "src/a.go", "a2")
	f.run(t, "tests", ledger.Passed, "C1")
	if rep := f.compute(t); rep.Level != 0 {
		t.Fatalf("tests passed without lint must not count: level %d", rep.Level)
	}
	f.run(t, "lint", ledger.Passed)
	rep := f.compute(t)
	if rep.Level != 2 {
		t.Fatalf("without review level must cap at 2, got %d", rep.Level)
	}
	f.review(t, ledger.Finding{ID: "f1", Severity: "blocking", Summary: "bug"})
	if rep := f.compute(t); rep.Level != 2 || len(rep.OpenBlocking) != 1 {
		t.Fatalf("open blocking finding must cap at 2: %+v", rep)
	}
	ledger.Append(f.t.Dir, ledger.Record{Type: ledger.TypeDecision, Kind: "finding_dismissed", Ref: "f1", By: "human:a"})
	rep = f.compute(t)
	if rep.Level != 3 || rep.Below {
		t.Fatalf("expected level 3, got %+v", rep)
	}
	if len(rep.Criteria) != 1 || !rep.Criteria[0].Verified {
		t.Fatalf("criterion C1 should be verified: %+v", rep.Criteria)
	}
}

func TestLatestRecordWins(t *testing.T) {
	f := setup(t)
	f.run(t, "lint", ledger.Passed)
	f.run(t, "lint", ledger.Failed)
	if rep := f.compute(t); rep.Level != 0 {
		t.Fatalf("a later failure must cancel an earlier pass: %d", rep.Level)
	}
}

func TestStaleOnCoveredChangeAndDef(t *testing.T) {
	f := setup(t)
	testutil.WriteFile(t, f.r.Root, "src/a.go", "a2")
	f.run(t, "lint", ledger.Passed)
	if rep := f.compute(t); rep.Level != 1 {
		t.Fatalf("level %d", rep.Level)
	}
	testutil.WriteFile(t, f.r.Root, "src/a.go", "a3")
	if rep := f.compute(t); rep.Level != 0 || rep.Checks[0].Status != Stale {
		t.Fatalf("edit must make lint stale: %+v", rep.Checks)
	}
	f.run(t, "lint", ledger.Passed)
	f.cfg.Checks[0].Run = "echo changed"
	if rep := f.compute(t); rep.Checks[0].Status != Stale {
		t.Fatal("changed check definition must make the record stale")
	}
}

func TestUnrelatedFileDoesNotStale(t *testing.T) {
	f := setup(t)
	testutil.WriteFile(t, f.r.Root, "src/a.go", "a2")
	f.run(t, "lint", ledger.Passed)
	testutil.WriteFile(t, f.r.Root, ".leadyard/tasks/T-1/notes.md", "x")
	if rep := f.compute(t); rep.Level != 1 {
		t.Fatalf("own state must not make evidence stale: %+v", rep.Checks)
	}
}

func TestEditedPreExistingTestNeedsAcceptance(t *testing.T) {
	f := setup(t)
	testutil.WriteFile(t, f.r.Root, "src/a_test.go", "t2")
	testutil.WriteFile(t, f.r.Root, "src/new_test.go", "n")
	f.run(t, "lint", ledger.Passed)
	f.run(t, "tests", ledger.Passed)
	f.review(t)
	rep := f.compute(t)
	if rep.Level != 2 || len(rep.UnacceptedEdit) != 1 || rep.UnacceptedEdit[0] != "src/a_test.go" {
		t.Fatalf("edited pre-existing test must block level 3: %+v", rep)
	}
	h, _ := f.r.HashFiles([]string{"src/a_test.go"})
	ledger.Append(f.t.Dir, ledger.Record{Type: ledger.TypeDecision, Kind: "test_edit_accepted", Ref: "src/a_test.go", Value: h["src/a_test.go"]})
	if rep := f.compute(t); rep.Level != 3 {
		t.Fatalf("accepted edit should allow level 3: %+v", rep)
	}
}
