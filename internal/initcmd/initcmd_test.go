package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redikultsev/leadyard/internal/testutil"
)

func TestInitIsIdempotentAndKeepsUserFiles(t *testing.T) {
	r := testutil.NewRepo(t, map[string]string{
		"AGENTS.md":             "# rules\n",
		"CLAUDE.md":             "# my notes\n",
		".claude/settings.json": `{"model":"opus","permissions":{"deny":["Read(./.env)"]}}`,
	})
	hooks := filepath.Join(r.Root, ".git", "hooks")
	os.MkdirAll(hooks, 0o755)
	os.WriteFile(filepath.Join(hooks, "commit-msg"), []byte("#!/bin/sh\necho mine\n"), 0o755)

	rep, err := Run(r, "leadyard")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "commit-msg") {
		t.Fatalf("a foreign git hook must be kept with a warning: %v", rep.Warnings)
	}
	b, _ := os.ReadFile(filepath.Join(r.Root, "CLAUDE.md"))
	if !strings.HasPrefix(string(b), "# my notes\n") || !strings.Contains(string(b), "@AGENTS.md") || !strings.Contains(string(b), "@.leadyard/agent.md") {
		t.Fatalf("CLAUDE.md: %q", b)
	}
	var s map[string]any
	b, _ = os.ReadFile(filepath.Join(r.Root, ".claude/settings.json"))
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s["model"] != "opus" || !strings.Contains(string(b), "Read(./.env)") || s["disableAutoMode"] != "disable" {
		t.Fatalf("settings lost user keys or missing ours: %s", b)
	}
	if mine, _ := os.ReadFile(filepath.Join(hooks, "commit-msg")); string(mine) != "#!/bin/sh\necho mine\n" {
		t.Fatal("foreign hook overwritten")
	}

	rep2, err := Run(r, "leadyard")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.Created)+len(rep2.Updated) != 0 {
		t.Fatalf("second init must change nothing: created %v updated %v", rep2.Created, rep2.Updated)
	}
}
