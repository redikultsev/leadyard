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

// The hook sees LEADYARD_BIN by Claude Code's precedence: the env of .claude/settings.local.json, then
// .claude/settings.json, then the user settings, then the environment (decision 70). Doctor's lookup probe
// must see the same, and say where it came from.
func TestLeadyardBin(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) envSource {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return envSource{name: name, path: p}
	}
	with := func(v string) string { return `{"env": {"LEADYARD_BIN": "` + v + `"}}` }
	local, project, user := write("local.json", with("/from/local")), write("project.json", with("/from/project")),
		write("user.json", with("/from/user"))
	other := write("other.json", `{"env": {"LEADYARD_AGENT": "claude-code"}}`)
	empty := write("empty.json", with(""))
	broken := write("broken.json", `{"env": `)
	notObject := write("not-object.json", `{"env": "x"}`)
	folder := envSource{name: "folder.json", path: filepath.Join(dir, "folder.json")}
	if err := os.Mkdir(folder.path, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := envSource{name: "missing.json", path: filepath.Join(dir, "missing.json")}
	t.Setenv("LEADYARD_BIN", "/from/env")

	cases := []struct {
		name       string
		sources    []envSource
		bin, from  string
		unreadable string // the path reported, if any
	}{
		{"local first", []envSource{local, project, user}, "/from/local", "local.json", ""},
		{"project over user", []envSource{missing, project, user}, "/from/project", "project.json", ""},
		{"user", []envSource{missing, other, user}, "/from/user", "user.json", ""},
		{"a file without it is passed over", []envSource{other, project}, "/from/project", "project.json", ""},
		// An empty value wins too: the hook gets it empty, as Claude Code resolves env per variable.
		{"an empty value wins", []envSource{empty, project}, "", "empty.json", ""},
		{"broken JSON is reported and skipped", []envSource{broken, user}, "/from/user", "user.json", broken.path},
		{"env that is not an object", []envSource{notObject, user}, "/from/user", "user.json", notObject.path},
		{"a read error", []envSource{folder, user}, "/from/user", "user.json", folder.path},
		{"environment last", []envSource{missing, other}, "/from/env", "the environment", ""},
	}
	for _, c := range cases {
		bin, from, unreadable := leadyardBin(c.sources)
		if bin != c.bin || from != c.from {
			t.Errorf("%s: got %q from %q; want %q from %q", c.name, bin, from, c.bin, c.from)
		}
		switch {
		case c.unreadable == "" && len(unreadable) != 0:
			t.Errorf("%s: unexpected unreadable %v", c.name, unreadable)
		case c.unreadable != "" && (len(unreadable) != 1 || unreadable[0].path != c.unreadable || unreadable[0].err == nil):
			t.Errorf("%s: unreadable %v, want %s with its error", c.name, unreadable, c.unreadable)
		}
	}
	t.Setenv("LEADYARD_BIN", "")
	if bin, from, _ := leadyardBin([]envSource{missing, other}); bin != "" || from != "the environment" {
		t.Errorf("empty in the environment: %q from %q", bin, from)
	}
	os.Unsetenv("LEADYARD_BIN")
	if bin, from, _ := leadyardBin([]envSource{missing, other}); bin != "" || from != "" {
		t.Errorf("none: %q from %q", bin, from)
	}
}

// The user settings live under CLAUDE_CONFIG_DIR when it is set, else under ~/.claude (%USERPROFILE% on Windows).
func TestSettingsSources(t *testing.T) {
	root := filepath.Join("repo")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	want := []string{filepath.Join(root, ".claude", "settings.local.json"), filepath.Join(root, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "settings.json")}
	got := settingsSources(root)
	if len(got) != len(want) {
		t.Fatalf("sources: %+v", got)
	}
	for i := range want {
		if got[i].path != want[i] {
			t.Errorf("source %d: %q, want %q", i, got[i].path, want[i])
		}
	}
	config := filepath.Join(t.TempDir(), "claude-config")
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	if got := settingsSources(root); got[2].path != filepath.Join(config, "settings.json") {
		t.Errorf("CLAUDE_CONFIG_DIR: %q", got[2].path)
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
