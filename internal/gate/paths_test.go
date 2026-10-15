package gate

import (
	"runtime"
	"testing"

	"github.com/redikultsev/leadyard/internal/config"
)

func TestWindowsPath(t *testing.T) {
	cases := []struct{ in, root, want string }{
		{`\Users\me\x`, `C:\repo`, `C:\Users\me\x`},
		{`/Users/me/x`, `C:/repo`, `C:\Users\me\x`},
		{`\\?\D:\repo\x`, `C:\repo`, `D:\repo\x`},
		{`//?/C:/repo/x`, `C:\repo`, `C:\repo\x`},
		{`/c/repo/x`, `D:\repo`, `C:\repo\x`},
		{`C:\repo\x`, `C:\repo`, `C:\repo\x`},
		{`src\a.go`, `C:\repo`, `src\a.go`},
		{`\\server\share\x`, `C:\repo`, `\\server\share\x`},
		{`\x`, `\\server\share\repo`, `\x`},
	}
	for _, c := range cases {
		if got := windowsPath(c.in, c.root); got != c.want {
			t.Errorf("windowsPath(%q, %q) = %q, want %q", c.in, c.root, got, c.want)
		}
	}
}

// Windows opens \x on the current drive and \\?\C:\x as C:\x: both still name the repository's files.
func TestDrivelessPathsOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("paths without a drive and \\\\?\\ paths are read only on Windows")
	}
	p := pol(config.DeliveryPR, "feat/x")
	cases := map[string]string{
		`\repo\.leadyard\config.yaml`:          Deny,
		`\repo\.git\config`:                    Deny,
		`\repo\src\a.go`:                       Allow,
		`\elsewhere\file`:                      Allow,
		`\\?\C:\repo\.leadyard\config.yaml`:    Deny,
		`//?/C:/repo/.leadyard/zones.yaml`:     Deny,
		`\\?\C:\repo\.github\workflows\ci.yml`: Ask,
		`\\?\C:\repo\src\a.go`:                 Allow,
	}
	for fp, want := range cases {
		got := Evaluate(HookInput{ToolName: "Write", ToolInput: map[string]any{"file_path": fp}}, p)
		if got.Verdict != want {
			t.Errorf("Write %s: got %s, want %s", fp, got.Verdict, want)
		}
	}
}
