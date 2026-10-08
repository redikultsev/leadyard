// Package initcmd installs leadyard into a repository: config templates, git
// attributes and ignores, the Claude Code adapter and git hooks (design §9.4).
// It never rewrites files the user wrote; it appends or merges.
package initcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zireaelq/leadyard/internal/config"
	"github.com/zireaelq/leadyard/internal/repo"
	"github.com/zireaelq/leadyard/skills"
)

// Report lists what init did.
type Report struct {
	Created  []string
	Updated  []string
	Skipped  []string
	Warnings []string
}

func (r *Report) created(p string) { r.Created = append(r.Created, p) }
func (r *Report) updated(p string) { r.Updated = append(r.Updated, p) }

const configTemplate = `# leadyard configuration (design §10). The agent never edits this file.
schema_version: 1
agents: [claude-code]

git:
  # none: the agent edits files only, you do every git write.
  # local: commits; branch: plus push of its branch; pr: plus draft PRs.
  delivery: pr
  base: main
  protected_branches: [main, master, develop, "release/*"]
  protected_paths: [".github/workflows/**", CODEOWNERS]

# Project checks mapped to evidence levels (design §5.4):
# 1 static (lint, types, build) · 2 agent tests · 3 the project's existing tests · 4 compare old vs new.
# tests: globs of test files; edits to pre-existing ones need your acceptance for level 3.
# inputs: extra files the check depends on besides the task's diff.
checks: []
#  - {id: lint,  level: 1, run: "make lint"}
#  - {id: tests, level: 3, run: "make test", tests: ["**/*_test.go"]}

# Required level per risk class (design §5.5).
class_levels: {1: 1, 2: 3, 3: 4}
`

const zonesTemplate = `# Areas where a change is sensitive (design §5.5). A diff touching a zone
# raises the task's class to at least the zone's class.
zones: []
#  - id: model-behavior
#    class: 3
#    why: "prompt changes alter outputs customers pay for"
#    paths: ["prompts/**"]
`

const agentInstructions = `# leadyard (managed by leadyard init; edits are overwritten)

This repository uses leadyard. The ` + "`leadyard`" + ` CLI owns task state and evidence.

- Start every task with the ` + "`leadyard`" + ` skill; after a context reset run ` + "`leadyard resume`" + `.
- "Done" is the level printed by ` + "`leadyard level`" + `, never your own judgment.
- Do not write .leadyard/config.yaml, .leadyard/zones.yaml or any evidence.jsonl; the gate blocks it.
- Follow the git delivery mode printed by ` + "`leadyard resume`" + `. In mode none you run no git writes.
- Do only what the request and the plan ask: no extra flags, env vars, files or dependencies.
`

// Run installs leadyard into the repository r.
func Run(r *repo.Repo, exe string) (Report, error) {
	var rep Report
	root := r.Root
	steps := []func() error{
		func() error { return writeIfAbsent(&rep, root, config.TeamFile, configTemplate) },
		func() error { return writeIfAbsent(&rep, root, config.ZonesFile, zonesTemplate) },
		func() error { return writeManaged(&rep, root, ".leadyard/agent.md", agentInstructions) },
		func() error {
			return appendLines(&rep, root, ".gitattributes", []string{".leadyard/**/*.jsonl merge=union"})
		},
		func() error {
			return appendLines(&rep, root, ".gitignore", []string{".leadyard/current",
				".leadyard/tasks/*/events.jsonl", ".leadyard/tasks/*/artifacts/", ".leadyard/config.local.yaml"})
		},
		func() error { return claudeMD(&rep, root) },
		func() error { return claudeSettings(&rep, root) },
		func() error { return hookScripts(&rep, root) },
		func() error { return installSkills(&rep, root) },
		func() error { return gitHooks(&rep, r, exe) },
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func writeIfAbsent(rep *Report, root, rel, content string) error {
	p := filepath.Join(root, rel)
	if _, err := os.Stat(p); err == nil {
		rep.Skipped = append(rep.Skipped, rel+" (exists)")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return err
	}
	rep.created(rel)
	return nil
}

func writeManaged(rep *Report, root, rel, content string) error {
	p := filepath.Join(root, rel)
	old, err := os.ReadFile(p)
	if err == nil && string(old) == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return err
	}
	if err == nil {
		rep.updated(rel)
	} else {
		rep.created(rel)
	}
	return nil
}

func appendLines(rep *Report, root, rel string, lines []string) error {
	p := filepath.Join(root, rel)
	old, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(old), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, l := range lines {
		if !have[l] {
			add = append(add, l)
		}
	}
	if len(add) == 0 {
		return nil
	}
	s := string(old)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	if s != "" {
		s += "\n"
	}
	s += "# leadyard\n" + strings.Join(add, "\n") + "\n"
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		return err
	}
	if old == nil {
		rep.created(rel)
	} else {
		rep.updated(rel)
	}
	return nil
}

func claudeMD(rep *Report, root string) error {
	lines := []string{"@.leadyard/agent.md"}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil {
		lines = append([]string{"@AGENTS.md"}, lines...)
	}
	return appendLines(rep, root, "CLAUDE.md", lines)
}

// Hook commands written into .claude/settings.json.
const (
	gateHook   = `"$CLAUDE_PROJECT_DIR"/.claude/hooks/leadyard-gate.sh`
	resumeHook = `"$CLAUDE_PROJECT_DIR"/.claude/hooks/leadyard-resume.sh`
)

func claudeSettings(rep *Report, root string) error {
	rel := ".claude/settings.json"
	p := filepath.Join(root, rel)
	settings := map[string]any{}
	old, err := os.ReadFile(p)
	if err == nil {
		if err := json.Unmarshal(old, &settings); err != nil {
			return fmt.Errorf("%s is not valid JSON; fix it and run init again: %w", rel, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	addHook(hooks, "PreToolUse", "Bash|Write|Edit|MultiEdit|NotebookEdit|EnterWorktree|mcp__.*", gateHook)
	addHook(hooks, "SessionStart", "startup|resume|compact", resumeHook)
	settings["hooks"] = hooks

	perms, _ := settings["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	deny := toStrings(perms["deny"])
	for _, rule := range []string{"Bash(git push --force *)", "Bash(git push -f *)", "Bash(git push * main)",
		"Bash(git push * master)", "Bash(gh pr merge *)", "Edit(./.leadyard/config.yaml)", "Edit(./.leadyard/zones.yaml)"} {
		if !containsStr(deny, rule) {
			deny = append(deny, rule)
		}
	}
	perms["deny"] = deny
	settings["permissions"] = perms
	settings["disableAutoMode"] = "disable"
	settings["autoMemoryEnabled"] = false
	attr, _ := settings["attribution"].(map[string]any)
	if attr == nil {
		attr = map[string]any{}
	}
	if _, ok := attr["commit"]; !ok {
		attr["commit"] = "" // the commit-msg hook adds Assisted-by instead (design §7.4)
	}
	settings["attribution"] = attr
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env["LEADYARD_AGENT"] = "claude-code"
	settings["env"] = env

	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if string(old) == string(b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return err
	}
	if old == nil {
		rep.created(rel)
	} else {
		rep.updated(rel)
	}
	return nil
}

func addHook(hooks map[string]any, event, matcher, command string) {
	list, _ := hooks[event].([]any)
	for _, g := range list {
		gm, _ := g.(map[string]any)
		inner, _ := gm["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); c == command {
				return
			}
		}
	}
	list = append(list, map[string]any{
		"matcher": matcher,
		"hooks":   []any{map[string]any{"type": "command", "command": command, "timeout": 10}},
	})
	hooks[event] = list
}

func toStrings(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// The wrappers fail closed: without the CLI every guarded call is blocked.
const gateScript = `#!/bin/sh
# Managed by leadyard init. Blocks the tool call (exit 2) when leadyard is missing.
if ! command -v leadyard >/dev/null 2>&1; then
  echo "leadyard is not installed or not on PATH; guarded actions are blocked. Install leadyard or remove this hook." >&2
  exit 2
fi
leadyard gate
code=$?
[ "$code" -eq 0 ] || exit 2
`

const resumeScript = `#!/bin/sh
# Managed by leadyard init. Prints the current task state into the session.
command -v leadyard >/dev/null 2>&1 || { echo "leadyard is not installed; task state is not loaded."; exit 0; }
leadyard resume --quiet || true
`

func hookScripts(rep *Report, root string) error {
	for name, content := range map[string]string{
		".claude/hooks/leadyard-gate.sh":   gateScript,
		".claude/hooks/leadyard-resume.sh": resumeScript,
	} {
		if err := writeManaged(rep, root, name, content); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(root, name), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func installSkills(rep *Report, root string) error {
	var names []string
	err := fs.WalkDir(skills.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(p) != "SKILL.md" {
			return nil
		}
		names = append(names, p)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, p := range names {
		b, err := skills.FS.ReadFile(p)
		if err != nil {
			return err
		}
		if err := writeManaged(rep, root, filepath.Join(".claude", "skills", p), string(b)); err != nil {
			return err
		}
	}
	return nil
}

func gitHooks(rep *Report, r *repo.Repo, exe string) error {
	if hp, _ := r.Git("config", "--get", "core.hooksPath"); strings.TrimSpace(hp) != "" {
		rep.Warnings = append(rep.Warnings, "core.hooksPath is set ("+strings.TrimSpace(hp)+"); add `leadyard githook pre-push` and `leadyard githook commit-msg` to your hook manager")
		return nil
	}
	dir, err := r.Git("rev-parse", "--git-path", "hooks")
	if err != nil {
		return err
	}
	hooksDir := strings.TrimSpace(dir)
	if !filepath.IsAbs(hooksDir) {
		hooksDir = filepath.Join(r.Root, hooksDir)
	}
	for _, name := range []string{"pre-push", "commit-msg"} {
		p := filepath.Join(hooksDir, name)
		script := fmt.Sprintf("#!/bin/sh\n# leadyard-managed\ncommand -v leadyard >/dev/null 2>&1 || exit 0\nexec leadyard githook %s \"$@\"\n", name)
		if old, err := os.ReadFile(p); err == nil {
			if strings.Contains(string(old), "leadyard-managed") {
				if string(old) == script {
					continue
				}
			} else {
				rep.Warnings = append(rep.Warnings, "git hook "+name+" exists and is not leadyard's; add `leadyard githook "+name+" \"$@\"` to it")
				continue
			}
		}
		if err := os.MkdirAll(hooksDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			return err
		}
		rep.created(".git/hooks/" + name)
	}
	return nil
}
