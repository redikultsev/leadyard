package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
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
		{"no sh", []string{"cmd/git.exe"}, "cmd/git.exe", ""},
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
