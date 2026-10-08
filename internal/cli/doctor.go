package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zireaelq/leadyard/internal/gate"
	"github.com/zireaelq/leadyard/internal/task"
)

type finding struct{ status, msg string }

func cmdDoctor(e *env, args []string) error {
	var res []finding
	ok := func(m string, a ...any) { res = append(res, finding{"ok  ", fmt.Sprintf(m, a...)}) }
	warn := func(m string, a ...any) { res = append(res, finding{"warn", fmt.Sprintf(m, a...)}) }
	fail := func(m string, a ...any) { res = append(res, finding{"FAIL", fmt.Sprintf(m, a...)}) }
	root := e.repo.Root

	bin, err := exec.LookPath("leadyard")
	if err != nil {
		fail("leadyard is not on PATH: hooks will block every guarded call")
	} else {
		ok("leadyard on PATH: %s", bin)
	}
	ok("config: delivery %s, base %s, %d checks, %d zones", e.cfg.Git.Delivery, e.cfg.Git.Base, len(e.cfg.Checks), len(e.cfg.Zones))
	if len(e.cfg.Checks) == 0 {
		warn("no checks configured: every task stays at level 0 (.leadyard/config.yaml → checks)")
	}

	var settings map[string]any
	if b, err := os.ReadFile(filepath.Join(root, ".claude/settings.json")); err != nil {
		fail(".claude/settings.json missing: run `leadyard init`")
	} else if err := json.Unmarshal(b, &settings); err != nil {
		fail(".claude/settings.json is not valid JSON: %v", err)
	} else {
		s := string(mustJSON(settings))
		check := func(cond bool, good, bad string) {
			if cond {
				ok("%s", good)
			} else {
				fail("%s", bad)
			}
		}
		check(strings.Contains(s, "leadyard-gate.sh"), "gate hook configured", "gate hook missing in .claude/settings.json")
		check(strings.Contains(s, "leadyard-resume.sh"), "resume hook configured", "resume hook missing in .claude/settings.json")
		check(settings["disableAutoMode"] == "disable", "auto mode disabled", "auto mode not disabled (disableAutoMode)")
		check(settings["autoMemoryEnabled"] == false, "auto memory off", "auto memory not turned off (autoMemoryEnabled)")
	}

	wrapper := filepath.Join(root, ".claude/hooks/leadyard-gate.sh")
	if st, err := os.Stat(wrapper); err != nil || st.Mode()&0o111 == 0 {
		fail("hook wrapper %s missing or not executable", wrapper)
	} else {
		push := `{"tool_name":"Bash","tool_input":{"command":"git push origin main"},"cwd":"` + root + `"}`
		if code := runWrapper(wrapper, root, push, "/usr/bin:/bin"); code == 2 {
			ok("wrapper blocks when leadyard is not on PATH")
		} else {
			fail("wrapper does not block without leadyard on PATH (exit %d)", code)
		}
		if bin != "" {
			path := filepath.Dir(bin) + ":/usr/bin:/bin"
			if code := runWrapper(wrapper, root, push, path); code == 2 {
				ok("gate blocks `git push origin main` through the wrapper")
			} else {
				fail("gate does not block `git push origin main` (exit %d)", code)
			}
			status := `{"tool_name":"Bash","tool_input":{"command":"git status"},"cwd":"` + root + `"}`
			start := time.Now()
			code := runWrapper(wrapper, root, status, path)
			if code == 0 {
				ok("gate allows `git status` (%s through the wrapper)", time.Since(start).Round(time.Millisecond))
			} else {
				fail("gate blocks `git status` (exit %d)", code)
			}
		}
	}

	// In-process latency of the policy.
	p, err := policyFor(root)
	if err != nil {
		fail("gate policy: %v", err)
	} else {
		cmds := []string{"git status", "git -C . push origin main", "go test ./... && git add -A && git commit -m x",
			"bash -c 'git push origin feat/x'", "cat .leadyard/tasks/x/evidence.jsonl | jq ."}
		var durs []time.Duration
		for i := 0; i < 200; i++ {
			c := cmds[i%len(cmds)]
			t0 := time.Now()
			gate.Evaluate(gate.HookInput{ToolName: "Bash", ToolInput: map[string]any{"command": c}}, p)
			durs = append(durs, time.Since(t0))
		}
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		p99 := durs[len(durs)*99/100]
		if p99 < 30*time.Millisecond {
			ok("gate policy p99 %s (budget 30ms)", p99.Round(time.Microsecond))
		} else {
			fail("gate policy p99 %s exceeds 30ms", p99)
		}
	}

	if hp, _ := e.repo.Git("config", "--get", "core.hooksPath"); strings.TrimSpace(hp) != "" {
		warn("core.hooksPath is set: make sure your hook manager calls `leadyard githook pre-push|commit-msg`")
	} else if dir, err := e.repo.Git("rev-parse", "--git-path", "hooks"); err == nil {
		hd := strings.TrimSpace(dir)
		if !filepath.IsAbs(hd) {
			hd = filepath.Join(root, hd)
		}
		if b, err := os.ReadFile(filepath.Join(hd, "pre-push")); err == nil && strings.Contains(string(b), "leadyard") {
			ok("git pre-push hook installed")
		} else {
			warn("git pre-push hook not installed: the push guard has one layer only")
		}
	}

	if id := task.Current(root); id != "" {
		if _, err := task.Load(root, id); err != nil {
			fail("current task %s: %v", id, err)
		} else {
			ok("current task %s", id)
		}
	}

	failed := false
	for _, f := range res {
		fmt.Fprintf(e.out, "[%s] %s\n", f.status, f.msg)
		if f.status == "FAIL" {
			failed = true
		}
	}
	if failed {
		return exitCode(1)
	}
	return nil
}

func runWrapper(wrapper, root, input, path string) int {
	cmd := exec.Command("/bin/sh", wrapper)
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + path, "HOME=" + os.Getenv("HOME"), "CLAUDE_PROJECT_DIR=" + root}
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return -1
	}
	return 0
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
