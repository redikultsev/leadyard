package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redikultsev/leadyard/internal/testutil"
)

// run executes a leadyard command in dir and returns stdout, stderr and the code.
func run(t *testing.T, dir string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	var out, errb bytes.Buffer
	code := Main(args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

func must(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, errs, code := run(t, dir, "", args...)
	if code != 0 {
		t.Fatalf("leadyard %v: exit %d\n%s%s", args, code, out, errs)
	}
	return out
}

// TestLoop runs a class-2 fix through the loop end to end.
func TestLoop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LEADYARD_AGENT", "")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "")
	r := testutil.NewRepo(t, map[string]string{
		"src/f.txt":      "to exclusive",
		"src/f_test.txt": "old test",
	})
	dir := r.Root
	must(t, dir, "init")
	testutil.WriteFile(t, dir, ".leadyard/config.yaml", `schema_version: 1
git: {delivery: none, base: main}
checks:
  - {id: lint,  level: 1, run: "test -f src/f.txt"}
  - {id: tests, level: 3, run: "grep -q inclusive src/f.txt", tests: ["src/*_test.txt"]}
`)
	testutil.Git(t, dir, "add", "-A")
	testutil.Git(t, dir, "commit", "-qm", "leadyard init")

	must(t, dir, "task", "new", "--id", "GH-1", "--title", "to inclusive", "--kind", "fix",
		"--request", "date_to must be inclusive")
	if _, _, code := run(t, dir, "", "decide", "class", "--value", "2", "--by", "agent"); code == 0 {
		t.Fatal("agent must not confirm the class")
	}
	must(t, dir, "transition", "start", "--by", "agent")
	must(t, dir, "decide", "class", "--value", "2", "--by", "human:a", "--reason", "ok")
	must(t, dir, "decide", "scope_approved", "--by", "human:a", "--reason", "ok")
	must(t, dir, "transition", "implement", "--by", "agent")
	if _, _, code := run(t, dir, "", "run", "tests", "--criteria", "C1"); code == 0 {
		t.Fatal("an unknown criterion must be refused")
	}
	tf := filepath.Join(dir, ".leadyard/tasks/GH-1/task.md")
	b, _ := os.ReadFile(tf)
	os.WriteFile(tf, []byte(strings.Replace(string(b), "## Plan", "- C1. date_to is inclusive\n\n## Plan", 1)), 0o644)

	// Red, then green.
	must(t, dir, "run", "lint")
	out := must(t, dir, "run", "tests", "--criteria", "C1")
	if !strings.Contains(out, "tests: failed") {
		t.Fatalf("expected a red run:\n%s", out)
	}
	testutil.WriteFile(t, dir, "src/f.txt", "to inclusive")
	if out := must(t, dir, "run", "lint"); !strings.Contains(out, "lint: passed") {
		t.Fatal(out)
	}
	must(t, dir, "run", "tests", "--criteria", "C1")
	if out := must(t, dir, "level"); !strings.Contains(out, "level 2 of required 3") {
		t.Fatalf("without review the level must be 2:\n%s", out)
	}
	must(t, dir, "transition", "verify", "--by", "agent")
	if _, _, code := run(t, dir, "", "transition", "approve_verdict", "--by", "human:a"); code == 0 {
		t.Fatal("approve_verdict below the requirement must fail")
	}

	in := must(t, dir, "review", "input")
	if !strings.Contains(in, "+to inclusive") {
		t.Fatalf("review input lacks the diff:\n%s", in)
	}
	reply := filepath.Join(t.TempDir(), "reply.md")
	os.WriteFile(reply, []byte("ok\n```leadyard-verdict\n{\"schema\":1,\"stage\":\"review\",\"status\":\"pass\",\"findings\":[]}\n```\n"), 0o644)
	must(t, dir, "review", "record", "--file", reply)
	if out := must(t, dir, "level"); !strings.Contains(out, "level 3 of required 3") {
		t.Fatalf("expected level 3:\n%s", out)
	}

	// Editing the code again makes the evidence stale.
	testutil.WriteFile(t, dir, "src/f.txt", "to inclusive!")
	if out := must(t, dir, "level"); !strings.Contains(out, "level 0") {
		t.Fatalf("edit after the runs must drop the level:\n%s", out)
	}
	testutil.WriteFile(t, dir, "src/f.txt", "to inclusive")
	must(t, dir, "transition", "approve_verdict", "--by", "human:a", "--reason", "good")
	if out := must(t, dir, "resume"); !strings.Contains(out, "status: ready_to_merge") || !strings.Contains(out, "delivery: none") {
		t.Fatalf("resume:\n%s", out)
	}
	if out := must(t, dir, "pr-body"); !strings.Contains(out, "C1 ") || !strings.Contains(out, "verified by tests") {
		t.Fatalf("pr body:\n%s", out)
	}

	// The gate, through the hook protocol.
	hook := `{"tool_name":"Bash","tool_input":{"command":"git add -A"},"cwd":"` + dir + `"}`
	if _, errs, code := run(t, dir, hook, "gate"); code != 2 || !strings.Contains(errs, "delivery mode") {
		t.Fatalf("gate must deny git add in mode none: %d %s", code, errs)
	}
	if _, _, code := run(t, dir, "not json", "gate"); code != 2 {
		t.Fatal("gate must fail closed on bad input")
	}
}
