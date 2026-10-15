package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/redikultsev/leadyard/internal/gate"
	"github.com/redikultsev/leadyard/internal/task"
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
	hook, hookErr := findHookEnv()
	// Windows has no exec bit; the shell runs the wrapper by name there.
	if st, err := os.Stat(wrapper); err != nil || (runtime.GOOS != "windows" && st.Mode()&0o111 == 0) {
		fail("hook wrapper %s missing or not executable", wrapper)
	} else if hookErr != nil {
		fail("no shell to run the hooks: %v", hookErr)
	} else {
		// The wrapper exits 2 both when leadyard is missing and when the gate denies, so the lookup probes use a
		// call the gate allows: 0 means leadyard was found and ran.
		push := hookInput("git push origin main", root)
		status := hookInput("git status", root)
		emptyHome, _ := os.MkdirTemp("", "leadyard-doctor-")
		defer os.RemoveAll(emptyHome)
		if code := hook.runWrapper(wrapper, root, status, hook.path(), emptyHome); code == 2 {
			ok("wrapper blocks when leadyard cannot be found")
		} else if code == 0 {
			warn("wrapper found a leadyard outside PATH (/opt/homebrew or /usr/local); the missing-binary block was not exercised")
		} else {
			fail("wrapper does not block without leadyard (exit %d)", code)
		}
		var extra []string // the missing-binary probe above gets none of this
		if b := leadyardBin(settings); b != "" {
			extra = append(extra, "LEADYARD_BIN="+b)
		}
		if code := hook.runWrapper(wrapper, root, status, hook.path(), userHome(), extra...); code == 0 {
			ok("hook finds leadyard without the shell PATH (as a desktop-launched agent would)")
		} else {
			fail("hook cannot find leadyard with a minimal PATH (exit %d): install into ~/go/bin, /opt/homebrew/bin or /usr/local/bin, or set LEADYARD_BIN in .claude/settings.json env", code)
		}
		if bin != "" {
			path := hook.path(filepath.Dir(bin))
			if code := hook.runWrapper(wrapper, root, push, path, userHome()); code == 2 {
				ok("gate blocks `git push origin main` through the wrapper")
			} else {
				fail("gate does not block `git push origin main` (exit %d)", code)
			}
			start := time.Now()
			code := hook.runWrapper(wrapper, root, status, path, userHome())
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

// hookEnv is how Claude Code runs a hook on this OS: the shell, and the system directories a minimal PATH
// keeps. On Windows that is the sh of Git for Windows and git's own directory (decision 66).
type hookEnv struct {
	sh  string
	sys []string
}

func findHookEnv() (hookEnv, error) {
	if runtime.GOOS != "windows" {
		return hookEnv{sh: "/bin/sh", sys: []string{"/usr/bin", "/bin"}}, nil
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return hookEnv{}, errors.New("git is not on PATH: install Git for Windows")
	}
	sh := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH")
	if sh == "" {
		// Never a bash found on PATH: System32\bash.exe starts WSL.
		if sh = gitSh(git); sh == "" {
			return hookEnv{}, fmt.Errorf("no sh.exe in the Git install of %s: reinstall Git for Windows or set CLAUDE_CODE_GIT_BASH_PATH", git)
		}
	} else if st, err := os.Stat(sh); err != nil || st.IsDir() {
		return hookEnv{}, fmt.Errorf("CLAUDE_CODE_GIT_BASH_PATH %s is not a file", sh)
	}
	return hookEnv{sh: sh, sys: []string{filepath.Dir(sh), filepath.Dir(git)}}, nil
}

// gitSh finds Git for Windows' sh.exe from its git.exe, which sits in cmd\, bin\ or mingw64\bin\ of the
// install; sh.exe is in usr\bin\ or bin\ of that install. "" for any other layout, such as a shim: an sh.exe
// found elsewhere is not Git's.
func gitSh(gitExe string) string {
	dir := filepath.Dir(gitExe)
	install := filepath.Dir(dir)
	switch strings.ToLower(filepath.Base(dir)) {
	case "cmd":
	case "bin":
		if strings.EqualFold(filepath.Base(install), "mingw64") {
			install = filepath.Dir(install)
		}
	default:
		return ""
	}
	for _, sh := range []string{filepath.Join(install, "usr", "bin", "sh.exe"), filepath.Join(install, "bin", "sh.exe")} {
		if st, err := os.Stat(sh); err == nil && !st.IsDir() {
			return sh
		}
	}
	return ""
}

// path is a minimal PATH: dirs, then the system directories, in this OS's list format. Git's sh reads a
// Windows-style list.
func (h hookEnv) path(dirs ...string) string {
	return strings.Join(append(append([]string{}, dirs...), h.sys...), string(os.PathListSeparator))
}

// hookInput is a PreToolUse event for a Bash command; a Windows root needs JSON escaping.
func hookInput(command, root string) string {
	return string(mustJSON(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": command}, "cwd": root}))
}

// userHome is HOME, or the OS's home directory where HOME is not set (PowerShell on Windows).
func userHome() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// leadyardBin is LEADYARD_BIN as the hook gets it: from the env of .claude/settings.json, else from the
// environment.
func leadyardBin(settings map[string]any) string {
	if env, ok := settings["env"].(map[string]any); ok {
		if b, ok := env["LEADYARD_BIN"].(string); ok && b != "" {
			return b
		}
	}
	return os.Getenv("LEADYARD_BIN")
}

func (h hookEnv) runWrapper(wrapper, root, input, path, home string, extra ...string) int {
	cmd := exec.Command(h.sh, wrapper)
	cmd.Dir = root
	cmd.Env = append([]string{"PATH=" + path, "HOME=" + home, "CLAUDE_PROJECT_DIR=" + root}, extra...)
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
