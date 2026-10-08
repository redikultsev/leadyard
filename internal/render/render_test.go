package render

import (
	"strings"
	"testing"

	"github.com/zireaelq/leadyard/internal/level"
	"github.com/zireaelq/leadyard/internal/task"
)

func TestResumeCapsAndShowsNext(t *testing.T) {
	tk, err := task.New(t.TempDir(), "T-1", "Date filter", "fix", "", "req", "")
	if err != nil {
		t.Fatal(err)
	}
	tk.AppendToSection("Notes", strings.Repeat("long note line\n", 2000))
	out := Resume(tk, level.Report{}, nil, "none")
	if len(out) > MaxResume {
		t.Fatalf("resume too long: %d", len(out))
	}
	if !strings.Contains(out, "next: intake") || !strings.Contains(out, "delivery: none") {
		t.Fatalf("resume misses next step or delivery:\n%s", out[:300])
	}
}

func TestLevelMarksBelow(t *testing.T) {
	out := Level(level.Report{Level: 2, Required: 3, Class: 2, Below: true, NotVerified: []string{"C1 x"}})
	if !strings.Contains(out, "BELOW REQUIREMENT") || !strings.Contains(out, "C1 x") {
		t.Fatal(out)
	}
}
