package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A Windows root has backslashes: the hook input must still be valid JSON that names that root.
func TestHookInput(t *testing.T) {
	root := `C:\Users\me\repo`
	var got struct {
		ToolName  string            `json:"tool_name"`
		ToolInput map[string]string `json:"tool_input"`
		Cwd       string            `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(hookInput("git push origin main", root)), &got); err != nil {
		t.Fatalf("hook input is not valid JSON: %v", err)
	}
	if got.ToolName != "Bash" || got.ToolInput["command"] != "git push origin main" || got.Cwd != root {
		t.Errorf("hook input: %+v", got)
	}
}

// Git for Windows keeps git.exe in cmd\, bin\ or mingw64\bin\ and sh.exe in usr\bin\ or bin\.
func TestGitSh(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		git   string
		want  string
	}{
		{"usr/bin first", []string{"cmd/git.exe", "usr/bin/sh.exe", "bin/sh.exe"}, "cmd/git.exe", "usr/bin/sh.exe"},
		{"bin only", []string{"mingw64/bin/git.exe", "bin/sh.exe"}, "mingw64/bin/git.exe", "bin/sh.exe"},
		{"git in bin", []string{"bin/git.exe", "usr/bin/sh.exe"}, "bin/git.exe", "usr/bin/sh.exe"},
		{"no sh", []string{"cmd/git.exe"}, "cmd/git.exe", ""},
		// Not Git's sh: above the install, or next to a shim.
		{"sh above the install", []string{"cmd/git.exe", "../usr/bin/sh.exe", "../bin/sh.exe"}, "cmd/git.exe", ""},
		{"shim", []string{"scoop/shims/git.exe", "bin/sh.exe", "scoop/bin/sh.exe"}, "scoop/shims/git.exe", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			install := filepath.Join(t.TempDir(), "Git")
			for _, f := range c.files {
				p := filepath.Join(install, filepath.FromSlash(f))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, nil, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			want := ""
			if c.want != "" {
				want = filepath.Join(install, filepath.FromSlash(c.want))
			}
			if got := gitSh(filepath.Join(install, filepath.FromSlash(c.git))); got != want {
				t.Errorf("gitSh = %q, want %q", got, want)
			}
		})
	}
}

func TestFindHookEnvUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix layout")
	}
	h, err := findHookEnv()
	if err != nil || h.sh != "/bin/sh" || strings.Join(h.sys, ":") != "/usr/bin:/bin" {
		t.Errorf("findHookEnv = %+v, %v", h, err)
	}
}

// On Windows the hook shell is CLAUDE_CODE_GIT_BASH_PATH or Git's own sh, never a bash or sh found on PATH:
// System32\bash.exe starts WSL (decision 66).
func TestFindHookEnvWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows layout")
	}
	dir := t.TempDir()
	touch := func(rel string) string {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	touch("wsl/bash.exe")
	touch("wsl/sh.exe")
	git := touch("Git/cmd/git.exe")
	gitSh := touch("Git/usr/bin/sh.exe")
	t.Setenv("PATH", filepath.Join(dir, "wsl")+";"+filepath.Dir(git))
	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", "")

	h, err := findHookEnv()
	if err != nil || h.sh != gitSh || strings.Join(h.sys, ";") != filepath.Dir(gitSh)+";"+filepath.Dir(git) {
		t.Errorf("Git's sh: findHookEnv = %+v, %v", h, err)
	}

	bash := touch("Git/bin/bash.exe")
	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", bash)
	if h, err := findHookEnv(); err != nil || h.sh != bash {
		t.Errorf("CLAUDE_CODE_GIT_BASH_PATH first: findHookEnv = %+v, %v", h, err)
	}

	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", filepath.Join(dir, "missing.exe"))
	if _, err := findHookEnv(); err == nil {
		t.Error("CLAUDE_CODE_GIT_BASH_PATH that is not a file: no error")
	}

	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", "")
	t.Setenv("PATH", filepath.Join(dir, "wsl"))
	if _, err := findHookEnv(); err == nil {
		t.Error("no git on PATH: no error")
	}
}

// The hook sees LEADYARD_BIN from the env of .claude/settings.json, else from the environment; doctor's
// lookup probe must see the same.
func TestLeadyardBin(t *testing.T) {
	t.Setenv("LEADYARD_BIN", "/from/env")
	settings := map[string]any{"env": map[string]any{"LEADYARD_BIN": "/from/settings"}}
	if got := leadyardBin(settings); got != "/from/settings" {
		t.Errorf("settings env: %q", got)
	}
	if got := leadyardBin(map[string]any{"env": map[string]any{"LEADYARD_AGENT": "claude-code"}}); got != "/from/env" {
		t.Errorf("environment: %q", got)
	}
	t.Setenv("LEADYARD_BIN", "")
	if got := leadyardBin(nil); got != "" {
		t.Errorf("none: %q", got)
	}
}

// The minimal PATH lists the given directories first, then the system's, in this OS's list format.
func TestHookEnvPath(t *testing.T) {
	h := hookEnv{sh: "sh", sys: []string{"s1", "s2"}}
	want := strings.Join([]string{"d", "s1", "s2"}, string(os.PathListSeparator))
	if got := h.path("d"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got := h.path(); got != strings.Join(h.sys, string(os.PathListSeparator)) {
		t.Errorf("path() = %q", got)
	}
}
