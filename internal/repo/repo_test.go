package repo_test

// Tests use the exported API only.

import (
	"reflect"
	"testing"

	. "github.com/redikultsev/leadyard/internal/repo"
	"github.com/redikultsev/leadyard/internal/testutil"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"src/**", "src/a/b.go", true},
		{"src/**", "src", true},
		{"**/*_test.go", "a/b/c_test.go", true},
		{"**/*_test.go", "c_test.go", true},
		{"*.go", "a/b.go", false},
		{"prompts/*.md", "prompts/x.md", true},
		{"prompts/*.md", "prompts/sub/x.md", false},
	}
	for _, c := range cases {
		if got := Match(c.pat, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pat, c.name, got, c.want)
		}
	}
}

func TestChangedFilesAndHashes(t *testing.T) {
	r := testutil.NewRepo(t, map[string]string{"a.go": "a", "b.go": "b"})
	testutil.WriteFile(t, r.Root, "a.go", "a2")
	testutil.WriteFile(t, r.Root, "new.go", "n")
	testutil.WriteFile(t, r.Root, ".leadyard/tasks/x/evidence.jsonl", "{}")
	got, err := r.ChangedFiles(r.BaseCommit("main"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.go", "new.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangedFiles = %v, want %v", got, want)
	}
	h1, err := r.HashFiles([]string{"a.go", "gone.go"})
	if err != nil {
		t.Fatal(err)
	}
	if h1["gone.go"] != "deleted" || h1["a.go"] == "" {
		t.Fatalf("HashFiles = %v", h1)
	}
	testutil.WriteFile(t, r.Root, "a.go", "a3")
	h2, _ := r.HashFiles([]string{"a.go"})
	if h1["a.go"] == h2["a.go"] {
		t.Fatal("hash did not change after edit")
	}
	base, _ := r.FilesAt(r.BaseCommit("main"))
	if !base["b.go"] || base["new.go"] {
		t.Fatalf("FilesAtBase = %v", base)
	}
}
