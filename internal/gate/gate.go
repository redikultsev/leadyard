// Package gate decides whether an agent tool call may run (design §7.1, §8.2).
// It parses shell commands into a syntax tree instead of matching prefixes, so
// "git -C . push" is seen as a push.
package gate

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/redikultsev/leadyard/internal/config"
	"github.com/redikultsev/leadyard/internal/repo"
)

// Verdicts.
const (
	Allow = "allow"
	Ask   = "ask"
	Deny  = "deny"
)

// Decision is the gate's answer with a reason that tells the agent what to do.
type Decision struct {
	Verdict string
	Reason  string
}

// HookInput is the part of a PreToolUse event the gate reads.
type HookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Cwd       string         `json:"cwd"`
}

// Policy is what the gate enforces.
type Policy struct {
	Root              string
	Delivery          string
	ProtectedBranches []string
	ProtectedPaths    []string
	CurrentBranch     string
}

func allow() Decision                  { return Decision{Verdict: Allow} }
func deny(f string, a ...any) Decision { return Decision{Deny, fmt.Sprintf(f, a...)} }
func ask(f string, a ...any) Decision  { return Decision{Ask, fmt.Sprintf(f, a...)} }
func worse(a, b Decision) Decision {
	rank := map[string]int{Allow: 0, Ask: 1, Deny: 2}
	if rank[b.Verdict] > rank[a.Verdict] {
		return b
	}
	return a
}

// Evaluate decides on one tool call.
func Evaluate(in HookInput, p Policy) Decision {
	switch in.ToolName {
	case "Bash":
		cmd, _ := in.ToolInput["command"].(string)
		return evalShell(cmd, p)
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		fp, _ := in.ToolInput["file_path"].(string)
		if fp == "" {
			fp, _ = in.ToolInput["notebook_path"].(string)
		}
		return evalWrite(fp, p)
	case "EnterWorktree":
		if p.Delivery == config.DeliveryNone {
			return deny("delivery mode none: the agent does not create worktrees; ask the human to set one up")
		}
		return ask("creating a worktree needs your confirmation (design §7.3)")
	}
	if strings.HasPrefix(in.ToolName, "mcp__") {
		n := strings.ToLower(in.ToolName)
		if (strings.Contains(n, "merge") || strings.Contains(n, "approve")) &&
			(strings.Contains(n, "pull") || strings.Contains(n, "pr") || strings.Contains(n, "github") || strings.Contains(n, "gitlab")) {
			return deny("merging and approving pull requests is a human decision")
		}
	}
	return allow()
}

// rel converts a path to a repository-relative slash path; "" if outside.
func rel(p Policy, file string) string {
	if file == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		file = windowsPath(file, p.Root)
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(p.Root, file)
	}
	r, err := filepath.Rel(p.Root, filepath.Clean(file))
	if err != nil || strings.HasPrefix(r, "..") {
		return ""
	}
	return filepath.ToSlash(r)
}

// fromGitBash reads a Git Bash drive path, /c or /c/..., as the Windows path it names, C:\... Claude Code's
// Bash tool on Windows runs in Git Bash, so commands name files this way (decision 66).
func fromGitBash(p string) (string, bool) {
	if len(p) < 2 || p[0] != '/' || !isLetter(p[1]) || (len(p) > 2 && p[2] != '/') {
		return "", false
	}
	rest := strings.TrimPrefix(p[2:], "/")
	return strings.ToUpper(p[1:2]) + `:\` + strings.ReplaceAll(rest, "/", `\`), true
}

func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

// windowsPath names a file the way Windows opens it: a Git Bash drive path as C:\..., \\?\C:\... as C:\...,
// and \... without a drive on the drive of root. Anything else comes back as given (decision 66).
func windowsPath(p, root string) string {
	if w, ok := fromGitBash(p); ok {
		return w
	}
	p = strings.ReplaceAll(p, "/", `\`)
	if len(p) >= 6 && strings.HasPrefix(p, `\\?\`) && isLetter(p[4]) && p[5] == ':' {
		return p[4:]
	}
	if strings.HasPrefix(p, `\`) && !strings.HasPrefix(p, `\\`) && len(root) >= 2 && isLetter(root[0]) && root[1] == ':' {
		return root[:2] + p
	}
	return p
}

// The checks below ignore letter case on every OS: Windows and macOS open .LEADYARD/config.yaml as
// .leadyard/config.yaml, and on a case-sensitive file system refusing such a name fails closed (decision 67).

// stateFile reports files only the CLI or a human may write.
func stateFile(r string) bool {
	r = strings.ToLower(r)
	if r == config.TeamFile || r == config.ZonesFile {
		return true
	}
	if strings.HasPrefix(r, ".leadyard/tasks/") {
		base := path.Base(r)
		return base == "evidence.jsonl" || base == "events.jsonl"
	}
	return false
}

func protectedPath(p Policy, r string) bool {
	for _, pat := range p.ProtectedPaths {
		if repo.Match(strings.ToLower(pat), strings.ToLower(r)) {
			return true
		}
	}
	return false
}

func evalWrite(file string, p Policy) Decision {
	r := rel(p, file)
	if r == "" {
		return allow()
	}
	if l := strings.ToLower(r); l == ".git" || strings.HasPrefix(l, ".git/") {
		return deny("the agent does not write inside .git/")
	}
	if stateFile(r) {
		return deny("%s is written only by the leadyard CLI or a human; use `leadyard run`, `leadyard decide` or ask the human", r)
	}
	if protectedPath(p, r) {
		return ask("%s is a protected path; changing it needs your confirmation", r)
	}
	return allow()
}

func evalShell(cmd string, p Policy) Decision {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return ask("leadyard could not parse this command (%v); confirm it yourself", err)
	}
	d := allow()
	syntax.Walk(f, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Stmt:
			for _, rd := range x.Redirs {
				if rd.Word == nil {
					continue
				}
				switch rd.Op {
				case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.RdrInOut, syntax.RdrAll, syntax.AppAll:
					if r := rel(p, lit(rd.Word)); r != "" {
						d = worse(d, evalWrite(r, p))
					}
				}
			}
		case *syntax.CallExpr:
			args := words(x.Args)
			if len(args) > 0 {
				d = worse(d, evalCall(args, p))
			}
		}
		return true
	})
	return d
}

const unknown = "\x00?"

func lit(w *syntax.Word) string {
	if s := w.Lit(); s != "" {
		return s
	}
	// Quoted literals.
	var b strings.Builder
	for _, part := range w.Parts {
		switch x := part.(type) {
		case *syntax.Lit:
			b.WriteString(x.Value)
		case *syntax.SglQuoted:
			b.WriteString(x.Value)
		case *syntax.DblQuoted:
			for _, q := range x.Parts {
				l, ok := q.(*syntax.Lit)
				if !ok {
					return unknown
				}
				b.WriteString(l.Value)
			}
		default:
			return unknown
		}
	}
	return b.String()
}

func words(ws []*syntax.Word) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, lit(w))
	}
	return out
}

var wrappers = map[string]bool{"command": true, "exec": true, "nohup": true, "time": true, "sudo": true, "builtin": true}

var readOnly = map[string]bool{"cat": true, "head": true, "tail": true, "less": true, "more": true, "grep": true,
	"rg": true, "jq": true, "wc": true, "ls": true, "stat": true, "file": true, "diff": true, "sort": true,
	"uniq": true, "awk": true, "echo": true, "printf": true, "test": true, "[": true, "leadyard": true}

func evalCall(args []string, p Policy) Decision {
	// Strip wrappers and env assignments.
	for len(args) > 0 {
		name := path.Base(args[0])
		if name == "env" {
			args = args[1:]
			for len(args) > 0 && (strings.Contains(args[0], "=") || strings.HasPrefix(args[0], "-")) {
				args = args[1:]
			}
			continue
		}
		if wrappers[name] {
			args = args[1:]
			continue
		}
		break
	}
	if len(args) == 0 {
		return allow()
	}
	name := path.Base(args[0])
	switch name {
	case "bash", "sh", "zsh", "dash":
		for i, a := range args {
			if a == "-c" && i+1 < len(args) {
				if args[i+1] == unknown {
					return ask("a shell script that is not a literal; confirm it yourself")
				}
				return evalShell(args[i+1], p)
			}
		}
		return allow()
	case "eval":
		script := strings.Join(args[1:], " ")
		if strings.Contains(script, unknown) {
			return ask("eval of a non-literal; confirm it yourself")
		}
		return evalShell(script, p)
	case "git":
		return evalGit(args[1:], p)
	case "gh":
		return evalGH(args[1:], p)
	}
	if !readOnly[name] {
		for _, a := range args[1:] {
			if r := rel(p, a); r != "" && stateFile(r) {
				return deny("%s is written only by the leadyard CLI or a human", r)
			}
		}
	}
	return allow()
}
