package ledger

import (
	"sort"
	"testing"
	"time"
)

func TestAppendRead(t *testing.T) {
	dir := t.TempDir()
	code := 0
	if _, err := Append(dir, Record{Type: TypeRun, Task: "GH-1", Check: "tests", Status: Passed, ExitCode: &code}); err != nil {
		t.Fatal(err)
	}
	if _, err := Append(dir, Record{Type: TypeDecision, Task: "GH-1", Kind: "scope_approved", By: "human:a"}); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].V != 1 || recs[0].ID == "" || *recs[0].ExitCode != 0 || recs[1].Kind != "scope_approved" {
		t.Fatalf("unexpected records: %+v", recs)
	}
}

func TestReadMissing(t *testing.T) {
	recs, err := Read(t.TempDir())
	if err != nil || recs != nil {
		t.Fatalf("missing ledger: %v %v", recs, err)
	}
}

func TestNewIDSortsByTime(t *testing.T) {
	a := NewID()
	time.Sleep(2 * time.Millisecond)
	b := NewID()
	if len(a) != 26 || len(b) != 26 {
		t.Fatalf("bad length: %q %q", a, b)
	}
	ids := []string{b, a}
	sort.Strings(ids)
	if ids[0] != a {
		t.Fatalf("ids do not sort by time: %v", ids)
	}
	if a == NewID() {
		t.Fatal("ids repeat")
	}
}
