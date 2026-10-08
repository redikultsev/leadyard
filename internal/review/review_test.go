package review

import (
	"strings"
	"testing"

	"github.com/zireaelq/leadyard/internal/ledger"
	"github.com/zireaelq/leadyard/internal/task"
	"github.com/zireaelq/leadyard/internal/testutil"
)

const reply = "Looks wrong.\n```leadyard-verdict\n{\"schema\":1,\"stage\":\"review\",\"status\":\"fail\",\"findings\":[{\"id\":\"F1\",\"severity\":\"blocking\",\"file\":\"a.go\",\"line\":3,\"summary\":\"date_to exclusive\"}]}\n```\n"

func TestParse(t *testing.T) {
	v, err := Parse("ignore ```leadyard-verdict\n{\"schema\":1,\"status\":\"pass\"}\n``` earlier\n" + reply)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "fail" || len(v.Findings) != 1 {
		t.Fatalf("last block must win: %+v", v)
	}
	for _, bad := range []string{"no block", "```leadyard-verdict\n{bad\n```", "```leadyard-verdict\n{\"schema\":1,\"status\":\"ok\"}\n```"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestRecordKeepsIDsAndCarriesOpen(t *testing.T) {
	r := testutil.NewRepo(t, map[string]string{"a.go": "a"})
	testutil.WriteFile(t, r.Root, "a.go", "b")
	tk, err := task.New(r.Root, "T-1", "x", "fix", "", "req", r.BaseCommit("main"))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Parse(reply)
	rec, err := Record(r, tk, nil, v)
	if err != nil {
		t.Fatal(err)
	}
	id := rec.Findings[0].ID
	if id == "F1" || !strings.HasPrefix(id, "f-") {
		t.Fatalf("model id must be replaced: %q", id)
	}
	recs, _ := ledger.Read(tk.Dir)
	// Round 2: reviewer says nothing about the previous finding: it stays open.
	v2, _ := Parse("```leadyard-verdict\n{\"schema\":1,\"stage\":\"review\",\"status\":\"pass\",\"findings\":[]}\n```")
	rec2, _ := Record(r, tk, recs, v2)
	if len(rec2.Findings) != 1 || rec2.Findings[0].ID != id {
		t.Fatalf("unreported previous finding must stay open: %+v", rec2.Findings)
	}
	recs, _ = ledger.Read(tk.Dir)
	v3, _ := Parse("```leadyard-verdict\n{\"schema\":1,\"stage\":\"review\",\"status\":\"pass\",\"findings\":[],\"previous\":[{\"id\":\"" + id + "\",\"state\":\"fixed\"}]}\n```")
	rec3, _ := Record(r, tk, recs, v3)
	if len(rec3.Findings) != 0 {
		t.Fatalf("fixed finding must close: %+v", rec3.Findings)
	}
	in, err := Input(r, tk, recs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in, id) || !strings.Contains(in, "+b") {
		t.Fatalf("input misses previous findings or diff:\n%s", in)
	}
}
