package gate

import (
	"testing"

	"github.com/zireaelq/leadyard/internal/config"
)

func pol(delivery, branch string) Policy {
	return Policy{Root: "/repo", Delivery: delivery, CurrentBranch: branch,
		ProtectedBranches: []string{"main", "master", "develop", "release/*"},
		ProtectedPaths:    []string{".github/workflows/**", "CODEOWNERS"}}
}

func bash(cmd string) HookInput {
	return HookInput{ToolName: "Bash", ToolInput: map[string]any{"command": cmd}}
}

func TestShell(t *testing.T) {
	cases := []struct {
		mode, branch, cmd, want string
	}{
		// Reads are always fine.
		{"none", "main", "git status && git diff main...HEAD | head", Allow},
		{"none", "main", "git -C . log --oneline -5", Allow},
		{"none", "main", "git branch -a", Allow},
		{"none", "main", "git config --get user.name", Allow},
		// Delivery mode none: no git writes at all.
		{"none", "feat/x", "git add a.go", Deny},
		{"none", "feat/x", "git commit -m x", Deny},
		{"none", "feat/x", "git fetch origin", Deny},
		{"none", "feat/x", "git stash", Deny},
		{"none", "feat/x", "git checkout -b agent/x", Deny},
		{"none", "feat/x", "cd sub && git -C .. commit -am x", Deny},
		// Local: commits allowed, push not.
		{"local", "feat/x", "git add a.go && git commit -m x", Allow},
		{"local", "feat/x", "git push origin feat/x", Deny},
		{"local", "feat/x", "git reset --hard HEAD~1", Ask},
		{"local", "feat/x", "git checkout -- .", Ask},
		{"local", "feat/x", "git worktree add ../wt", Ask},
		// Branch / PR: own branch only.
		{"pr", "feat/x", "git push origin feat/x", Allow},
		{"pr", "feat/x", "git push", Allow},
		{"pr", "main", "git push", Deny},
		{"pr", "feat/x", "git -C . push origin main", Deny},
		{"pr", "feat/x", "git push origin HEAD:main", Deny},
		{"pr", "feat/x", "git push origin release/1.2", Deny},
		{"pr", "feat/x", "git push --force origin feat/x", Deny},
		{"pr", "feat/x", "git push origin +feat/x", Deny},
		{"pr", "feat/x", "git push --force-with-lease origin feat/x", Ask},
		{"pr", "feat/x", "git push origin :feat/old", Deny},
		{"pr", "feat/x", "git push --tags", Deny},
		{"pr", "feat/x", "git push origin $BR", Ask},
		{"pr", "feat/x", "git commit --no-verify -m x", Deny},
		{"pr", "feat/x", "git -c core.hooksPath=/dev/null commit -m x", Deny},
		{"pr", "feat/x", "git config user.email x@y", Deny},
		{"pr", "feat/x", "git remote set-url origin x", Deny},
		{"pr", "feat/x", "git tag v1", Deny},
		{"pr", "feat/x", "env GIT_DIR=.git git push origin main", Deny},
		{"pr", "feat/x", "sudo git push origin main", Deny},
		{"pr", "feat/x", "bash -c 'echo hi'", Allow},
		{"pr", "feat/x", "bash -c 'git push origin main'", Deny},
		{"pr", "feat/x", "eval git push origin main", Deny},
		{"none", "feat/x", "sh -c \"git commit -m x\"", Deny},
		// gh.
		{"pr", "feat/x", "gh pr create --draft --title x --body y", Allow},
		{"pr", "feat/x", "gh pr create --title x", Deny},
		{"branch", "feat/x", "gh pr create --draft", Deny},
		{"pr", "feat/x", "gh pr merge 12 --squash", Deny},
		{"pr", "feat/x", "gh pr review 12 --approve", Deny},
		{"pr", "feat/x", "gh api -X PUT repos/o/r/pulls/12/merge", Deny},
		{"pr", "feat/x", "gh api repos/o/r/pulls/12", Allow},
		// Protected state files.
		{"pr", "feat/x", "echo '{}' >> .leadyard/tasks/T-1/evidence.jsonl", Deny},
		{"pr", "feat/x", "sed -i '' 's/failed/passed/' .leadyard/tasks/T-1/evidence.jsonl", Deny},
		{"pr", "feat/x", "cat .leadyard/tasks/T-1/evidence.jsonl", Allow},
		{"pr", "feat/x", "leadyard run tests", Allow},
		// Unparseable.
		{"pr", "feat/x", "echo 'unterminated", Ask},
	}
	for _, c := range cases {
		got := Evaluate(bash(c.cmd), pol(c.mode, c.branch))
		if got.Verdict != c.want {
			t.Errorf("[%s on %s] %q: got %s (%s), want %s", c.mode, c.branch, c.cmd, got.Verdict, got.Reason, c.want)
		}
		if got.Verdict != Allow && got.Reason == "" {
			t.Errorf("%q: %s without a reason", c.cmd, got.Verdict)
		}
	}
}

func TestFileTools(t *testing.T) {
	p := pol(config.DeliveryPR, "feat/x")
	cases := map[string]string{
		"/repo/src/a.go":                           Allow,
		"/repo/.leadyard/tasks/T-1/task.md":        Allow,
		"/repo/.leadyard/tasks/T-1/evidence.jsonl": Deny,
		"/repo/.leadyard/config.yaml":              Deny,
		"/repo/.git/hooks/pre-push":                Deny,
		"/repo/.github/workflows/ci.yml":           Ask,
		"/elsewhere/file":                          Allow,
	}
	for fp, want := range cases {
		got := Evaluate(HookInput{ToolName: "Write", ToolInput: map[string]any{"file_path": fp}}, p)
		if got.Verdict != want {
			t.Errorf("Write %s: got %s, want %s", fp, got.Verdict, want)
		}
	}
	if d := Evaluate(HookInput{ToolName: "EnterWorktree"}, pol("none", "x")); d.Verdict != Deny {
		t.Errorf("EnterWorktree in none: %s", d.Verdict)
	}
	if d := Evaluate(HookInput{ToolName: "mcp__github__merge_pull_request"}, p); d.Verdict != Deny {
		t.Errorf("MCP merge: %s", d.Verdict)
	}
	if d := Evaluate(HookInput{ToolName: "Read", ToolInput: map[string]any{"file_path": "/repo/.leadyard/tasks/T-1/evidence.jsonl"}}, p); d.Verdict != Allow {
		t.Errorf("Read must be allowed: %s", d.Verdict)
	}
}
