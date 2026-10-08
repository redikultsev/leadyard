package task

import (
	"strings"
	"testing"
)

func TestNewLoadSections(t *testing.T) {
	root := t.TempDir()
	tk, err := New(root, "GH-142", "Date filter", "fix", "https://x/142", "Add date_from and date_to.", "abc")
	if err != nil {
		t.Fatal(err)
	}
	tk.AppendToSection("Acceptance criteria", "- C1. Orders created on date_to are included.")
	tk.AppendToSection("Acceptance criteria", "- C2. Invalid dates give 400.")
	tk.AppendToSection("Plan", "- [x] Parse params")
	tk.AppendToSection("Plan", "- [ ] Add tests")
	tk.AppendToSection("Decisions", "- d1")
	tk.Meta.Status = "clarifying"
	if err := tk.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root, "GH-142")
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Status != "clarifying" || got.Meta.Kind != "fix" || got.Meta.Attempt != 1 {
		t.Fatalf("meta: %+v", got.Meta)
	}
	cr := got.Criteria()
	if len(cr) != 2 || cr[1].ID != "C2" || cr[1].Text != "Invalid dates give 400." {
		t.Fatalf("criteria: %+v", cr)
	}
	pl := got.Plan()
	if len(pl) != 2 || !pl[0].Done || pl[1].Done {
		t.Fatalf("plan: %+v", pl)
	}
	if !strings.Contains(got.Section("Request (verbatim)"), "date_from") {
		t.Fatalf("request section: %q", got.Section("Request (verbatim)"))
	}
	if strings.Contains(got.Section("Plan"), "<!--") {
		t.Fatal("comments must be stripped")
	}
	// Decisions stays last and keeps order.
	if !strings.HasSuffix(strings.TrimSpace(got.Body), "- d1") {
		t.Fatalf("decision not at end: %q", got.Body)
	}
}

func TestIDValidationAndCurrent(t *testing.T) {
	root := t.TempDir()
	if _, err := New(root, "../x", "t", "", "", "", ""); err == nil {
		t.Fatal("path traversal id accepted")
	}
	tk, err := New(root, "", "t", "", "", "", "")
	if err != nil || len(tk.Meta.ID) != 26 {
		t.Fatalf("ulid id: %v %v", tk, err)
	}
	if err := SetCurrent(root, tk.Meta.ID); err != nil {
		t.Fatal(err)
	}
	if Current(root) != tk.Meta.ID {
		t.Fatal("current not stored")
	}
	ids, _ := List(root)
	if len(ids) != 1 {
		t.Fatalf("list: %v", ids)
	}
}
