package gate

import (
	"strings"

	"github.com/zireaelq/leadyard/internal/config"
	"github.com/zireaelq/leadyard/internal/repo"
)

var gitRead = map[string]bool{"status": true, "diff": true, "log": true, "show": true, "blame": true,
	"ls-files": true, "ls-tree": true, "rev-parse": true, "rev-list": true, "describe": true, "grep": true,
	"cat-file": true, "shortlog": true, "show-ref": true, "for-each-ref": true, "merge-base": true,
	"name-rev": true, "check-ignore": true, "var": true, "help": true, "version": true, "count-objects": true,
	"range-diff": true, "whatchanged": true, "annotate": true}

func hasAny(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f || (strings.HasPrefix(f, "--") && strings.HasPrefix(a, f+"=")) {
				return true
			}
		}
	}
	return false
}

func positional(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

func (p Policy) protected(branch string) bool {
	b := strings.TrimPrefix(branch, "refs/heads/")
	return repo.MatchAny(p.ProtectedBranches, b)
}

func needs(p Policy, mode, what string) (Decision, bool) {
	if !config.AllowsAtLeast(p.Delivery, mode) {
		return deny("delivery mode %q does not allow the agent to %s; leave git writes to the human", p.Delivery, what), true
	}
	return Decision{}, false
}

func evalGit(args []string, p Policy) Decision {
	// Global options.
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		a := args[0]
		switch {
		case a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "--namespace":
			if a == "-c" && len(args) > 1 && strings.HasPrefix(strings.ToLower(args[1]), "core.hookspath") {
				return deny("changing core.hooksPath is not allowed")
			}
			if len(args) < 2 {
				return ask("incomplete git command")
			}
			args = args[2:]
		default:
			args = args[1:]
		}
	}
	if len(args) == 0 {
		return allow()
	}
	sub, rest := args[0], args[1:]
	if sub == unknown {
		return ask("git subcommand is not a literal; confirm it yourself")
	}
	if hasAny(rest, "--no-verify") {
		return deny("--no-verify skips the project's git hooks and is not allowed")
	}
	if gitRead[sub] {
		return allow()
	}
	switch sub {
	case "branch":
		if len(positional(rest)) == 0 || hasAny(rest, "-l", "--list", "-a", "-r", "-v", "-vv", "--show-current", "--contains", "--merged", "--no-merged") && !hasAny(rest, "-d", "-D", "-m", "-M", "--delete", "--move") {
			return allow()
		}
		if d, ok := needs(p, config.DeliveryLocal, "create or change branches"); ok {
			return d
		}
		if hasAny(rest, "-d", "-D", "-m", "-M", "--delete", "--move", "-f", "--force") {
			return ask("deleting, renaming or force-moving a branch needs your confirmation")
		}
		return allow()
	case "tag":
		if len(positional(rest)) == 0 || hasAny(rest, "-l", "--list") {
			return allow()
		}
		return deny("creating or deleting tags is a human action")
	case "remote":
		ps := positional(rest)
		if len(ps) == 0 || ps[0] == "show" || ps[0] == "get-url" {
			return allow()
		}
		return deny("changing remotes is not allowed")
	case "config":
		if hasAny(rest, "--get", "--get-all", "--get-regexp", "--list", "-l") || (len(rest) > 0 && rest[0] == "get") {
			return allow()
		}
		return deny("changing git config is not allowed")
	case "stash":
		ps := positional(rest)
		if len(ps) > 0 && (ps[0] == "list" || ps[0] == "show") {
			return allow()
		}
		if d, ok := needs(p, config.DeliveryLocal, "stash changes"); ok {
			return d
		}
		if len(ps) > 0 && (ps[0] == "drop" || ps[0] == "clear") {
			return ask("dropping a stash loses work; confirm it yourself")
		}
		return allow()
	case "worktree":
		ps := positional(rest)
		if len(ps) > 0 && ps[0] == "list" {
			return allow()
		}
		if d, ok := needs(p, config.DeliveryLocal, "manage worktrees"); ok {
			return d
		}
		return ask("worktree changes need your confirmation (design §7.3)")
	case "fetch":
		if d, ok := needs(p, config.DeliveryLocal, "fetch"); ok {
			return d
		}
		return allow()
	case "pull":
		if d, ok := needs(p, config.DeliveryLocal, "pull"); ok {
			return d
		}
		return ask("pull merges remote changes into your tree; confirm it yourself")
	case "reset":
		if d, ok := needs(p, config.DeliveryLocal, "reset"); ok {
			return d
		}
		if hasAny(rest, "--hard", "--merge", "--keep") {
			return ask("reset --hard can lose work; confirm it yourself")
		}
		return allow()
	case "clean":
		if d, ok := needs(p, config.DeliveryLocal, "clean the tree"); ok {
			return d
		}
		return ask("git clean deletes files; confirm it yourself")
	case "checkout":
		if d, ok := needs(p, config.DeliveryLocal, "check out"); ok {
			return d
		}
		if hasAny(rest, "--", ".", "-f", "--force", "-p", "--patch") {
			return ask("this discards changes in the working tree; confirm it yourself")
		}
		return allow()
	case "restore":
		if d, ok := needs(p, config.DeliveryLocal, "restore files"); ok {
			return d
		}
		if hasAny(rest, "--staged", "-S") && !hasAny(rest, "--worktree", "-W") {
			return allow()
		}
		return ask("this discards changes in the working tree; confirm it yourself")
	case "switch", "add", "commit", "mv", "rm", "merge", "rebase", "cherry-pick", "revert", "am", "apply", "notes":
		if d, ok := needs(p, config.DeliveryLocal, sub); ok {
			return d
		}
		return allow()
	case "push":
		return evalPush(rest, p)
	case "filter-branch", "filter-repo", "update-ref", "symbolic-ref", "replace":
		return deny("rewriting refs or history with %s is not allowed", sub)
	case "gc", "prune", "repack", "submodule", "lfs", "sparse-checkout", "init", "clone", "bisect":
		return ask("git %s needs your confirmation", sub)
	}
	return ask("leadyard does not know `git %s`; confirm it yourself", sub)
}

func evalPush(rest []string, p Policy) Decision {
	if d, ok := needs(p, config.DeliveryBranch, "push"); ok {
		return d
	}
	if hasAny(rest, "--mirror", "--delete", "-d", "--tags", "--all", "--prune") {
		return deny("this push form (mirror, delete, tags, all, prune) is not allowed")
	}
	if hasAny(rest, "--force", "-f") {
		return deny("force push is not allowed; --force-with-lease on your own branch needs confirmation")
	}
	lease := false
	for _, a := range rest {
		if strings.HasPrefix(a, "--force-with-lease") || strings.HasPrefix(a, "--force-if-includes") {
			lease = true
		}
	}
	ps := positional(rest)
	var refspecs []string
	if len(ps) > 1 {
		refspecs = ps[1:]
	}
	if len(refspecs) == 0 {
		if p.CurrentBranch == "" || p.protected(p.CurrentBranch) {
			return deny("pushing %q is not allowed; push your task branch", nonEmpty(p.CurrentBranch, "HEAD"))
		}
	}
	for _, rs := range refspecs {
		if rs == unknown {
			return ask("the push target is not a literal; confirm it yourself")
		}
		if strings.HasPrefix(rs, "+") {
			return deny("forced refspec %q is not allowed", rs)
		}
		dst := rs
		if i := strings.Index(rs, ":"); i >= 0 {
			if i == 0 {
				return deny("deleting a remote branch is not allowed")
			}
			dst = rs[i+1:]
		}
		if dst == "HEAD" {
			dst = p.CurrentBranch
		}
		if dst == "" || p.protected(dst) {
			return deny("pushing to %q is not allowed; push your task branch and open a draft PR", nonEmpty(dst, rs))
		}
	}
	if lease {
		return ask("rewriting a published branch needs your confirmation")
	}
	return allow()
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func evalGH(args []string, p Policy) Decision {
	if len(args) == 0 {
		return allow()
	}
	group := args[0]
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	rest := []string{}
	if len(args) > 2 {
		rest = args[2:]
	}
	switch group {
	case "pr":
		switch sub {
		case "merge":
			return deny("merging a pull request is a human decision")
		case "review":
			if hasAny(rest, "--approve", "-a") {
				return deny("approving a pull request is a human decision")
			}
			return allow()
		case "ready":
			return ask("marking a draft ready for review needs your confirmation")
		case "create":
			if d, ok := needs(p, config.DeliveryPR, "open pull requests"); ok {
				return d
			}
			if !hasAny(rest, "--draft", "-d") {
				return deny("open the pull request as a draft: add --draft")
			}
			return allow()
		case "close", "reopen", "edit", "comment":
			if d, ok := needs(p, config.DeliveryPR, "change pull requests"); ok {
				return d
			}
			return allow()
		}
		return allow()
	case "repo":
		switch sub {
		case "edit", "delete", "rename", "archive", "unarchive":
			return deny("changing repository settings is a human action")
		}
	case "secret", "variable", "ruleset":
		if sub != "list" && sub != "view" && sub != "" {
			return deny("changing %s is a human action", group)
		}
	case "api":
		all := strings.ToLower(strings.Join(args, " "))
		method := "GET"
		for i, a := range args {
			if (a == "-X" || a == "--method") && i+1 < len(args) {
				method = strings.ToUpper(args[i+1])
			}
		}
		if method != "GET" && (strings.Contains(all, "/merge") || strings.Contains(all, "protection") ||
			strings.Contains(all, "rulesets") || strings.Contains(all, "/reviews") || strings.Contains(all, "/git/refs")) {
			return deny("this API call changes branches, merges or reviews; that is a human action")
		}
	}
	return allow()
}
