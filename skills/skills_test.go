package skills

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Static check of the Agent Skills front matter: name matches the folder,
// description present, only portable fields.
func TestFrontMatter(t *testing.T) {
	portable := map[string]bool{"name": true, "description": true, "license": true, "compatibility": true, "metadata": true}
	n := 0
	fs.WalkDir(FS, ".", func(p string, d fs.DirEntry, err error) error {
		if d.IsDir() || path.Base(p) != "SKILL.md" {
			return nil
		}
		n++
		b, _ := FS.ReadFile(p)
		s := string(b)
		if !strings.HasPrefix(s, "---\n") {
			t.Errorf("%s: no front matter", p)
			return nil
		}
		fm := s[4 : 4+strings.Index(s[4:], "\n---\n")]
		fields := map[string]string{}
		for _, l := range strings.Split(fm, "\n") {
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			fields[k] = strings.TrimSpace(v)
			if !portable[k] {
				t.Errorf("%s: non-portable field %q", p, k)
			}
		}
		dir := path.Dir(p)
		if fields["name"] != dir || !nameRe.MatchString(dir) {
			t.Errorf("%s: name %q must equal the folder %q", p, fields["name"], dir)
		}
		if len(fields["description"]) < 40 || len(fields["description"]) > 1024 {
			t.Errorf("%s: description length %d", p, len(fields["description"]))
		}
		return nil
	})
	if n < 8 {
		t.Fatalf("expected at least 8 skills, found %d", n)
	}
}
