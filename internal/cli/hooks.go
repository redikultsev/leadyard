package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redikultsev/leadyard/internal/config"
	"github.com/redikultsev/leadyard/internal/gate"
	"github.com/redikultsev/leadyard/internal/repo"
	"github.com/redikultsev/leadyard/internal/task"
)

// policyFor builds the gate policy for the repository containing dir.
func policyFor(dir string) (gate.Policy, error) {
	cfg := config.Default()
	p := gate.Policy{Root: dir}
	r, err := repo.Open(dir)
	if err == nil {
		p.Root = r.Root
		if cfg, err = config.Load(r.Root); err != nil {
			return p, err
		}
		p.CurrentBranch = r.CurrentBranch()
		if id := task.Current(r.Root); id != "" {
			if t, err := task.Load(r.Root, id); err == nil && t.Meta.Delivery != "" &&
				config.AllowsAtLeast(cfg.Git.Delivery, t.Meta.Delivery) {
				cfg.Git.Delivery = t.Meta.Delivery
			}
		}
	}
	p.Delivery = cfg.Git.Delivery
	p.ProtectedBranches = cfg.Git.ProtectedBranches
	p.ProtectedPaths = cfg.Git.ProtectedPaths
	return p, nil
}

// runGate is the PreToolUse hook. Any failure blocks (exit 2).
func runGate(in io.Reader, out, errw io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(errw, "leadyard gate crashed; blocking:", r)
			code = 2
		}
	}()
	var hi gate.HookInput
	if err := json.NewDecoder(in).Decode(&hi); err != nil {
		fmt.Fprintln(errw, "leadyard gate: cannot read the tool call; blocking:", err)
		return 2
	}
	dir := hi.Cwd
	if d := os.Getenv("CLAUDE_PROJECT_DIR"); d != "" {
		dir = d
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	p, err := policyFor(dir)
	if err != nil {
		fmt.Fprintln(errw, "leadyard gate: configuration error; blocking until it is fixed:", err)
		return 2
	}
	d := gate.Evaluate(hi, p)
	logEvent(p.Root, hi, d)
	switch d.Verdict {
	case gate.Deny:
		fmt.Fprintln(errw, "leadyard: "+d.Reason)
		return 2
	case gate.Ask:
		json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PreToolUse", "permissionDecision": "ask",
			"permissionDecisionReason": "leadyard: " + d.Reason}})
	}
	return 0
}

// logEvent appends gate denials and questions to the current task's local event log.
func logEvent(root string, hi gate.HookInput, d gate.Decision) {
	if d.Verdict == gate.Allow {
		return
	}
	id := task.Current(root)
	if id == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(task.Dir(root, id), "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(map[string]any{"at": time.Now().UTC(), "event": "gate", "verdict": d.Verdict,
		"tool": hi.ToolName, "reason": d.Reason})
}

func agentEnv() string {
	if a := os.Getenv("LEADYARD_AGENT"); a != "" {
		return a
	}
	if os.Getenv("CLAUDE_CODE_CHILD_SESSION") == "1" {
		return "claude-code"
	}
	return ""
}

const zeroSHA = "0000000000000000000000000000000000000000"

// runGitHook implements git hooks. They act only when an agent runs git, so
// human pushes and commits are never blocked.
func runGitHook(args []string, in io.Reader, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "usage: leadyard githook pre-push|commit-msg")
		return 2
	}
	agent := agentEnv()
	if agent == "" {
		return 0
	}
	wd, _ := os.Getwd()
	r, err := repo.Open(wd)
	if err != nil {
		fmt.Fprintln(errw, "leadyard githook:", err)
		return 1
	}
	switch args[0] {
	case "commit-msg":
		if len(args) < 2 {
			return 0
		}
		b, err := os.ReadFile(args[1])
		if err != nil {
			fmt.Fprintln(errw, "leadyard githook:", err)
			return 1
		}
		msg := string(b)
		trailer := "Assisted-by: " + agent
		if strings.Contains(msg, "Assisted-by:") {
			return 0
		}
		msg = strings.TrimRight(msg, "\n") + "\n\n" + trailer + "\n"
		if err := os.WriteFile(args[1], []byte(msg), 0o644); err != nil {
			fmt.Fprintln(errw, "leadyard githook:", err)
			return 1
		}
		return 0
	case "pre-push":
		p, err := policyFor(r.Root)
		if err != nil {
			fmt.Fprintln(errw, "leadyard pre-push: configuration error; blocking:", err)
			return 1
		}
		if !config.AllowsAtLeast(p.Delivery, config.DeliveryBranch) {
			fmt.Fprintf(errw, "leadyard pre-push: delivery mode %q does not allow the agent to push\n", p.Delivery)
			return 1
		}
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) != 4 {
				continue
			}
			localSHA, remoteRef, remoteSHA := f[1], f[2], f[3]
			branch := strings.TrimPrefix(remoteRef, "refs/heads/")
			if strings.HasPrefix(remoteRef, "refs/tags/") {
				fmt.Fprintln(errw, "leadyard pre-push: the agent does not push tags")
				return 1
			}
			if repo.MatchAny(p.ProtectedBranches, branch) {
				fmt.Fprintf(errw, "leadyard pre-push: the agent does not push to %s\n", branch)
				return 1
			}
			if localSHA == zeroSHA {
				fmt.Fprintf(errw, "leadyard pre-push: the agent does not delete remote branches (%s)\n", branch)
				return 1
			}
			if remoteSHA != zeroSHA {
				if _, err := r.Git("merge-base", "--is-ancestor", remoteSHA, localSHA); err != nil {
					fmt.Fprintf(errw, "leadyard pre-push: %s is not a fast-forward; rewriting a published branch needs the human\n", branch)
					return 1
				}
			}
		}
		return 0
	}
	fmt.Fprintf(errw, "leadyard githook: unknown hook %q\n", args[0])
	return 2
}
