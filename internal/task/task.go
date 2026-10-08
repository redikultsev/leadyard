// Package task manages task folders: .leadyard/tasks/<id>/task.md with front
// matter written only by the CLI and Markdown sections (design §5.1).
package task

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zireaelq/leadyard/internal/ledger"
)

// Statuses fixed in the core (design §5.2).
var Statuses = []string{"new", "clarifying", "planning", "in_progress", "verifying",
	"waiting_human", "ready_to_merge", "merged", "deployed", "closed", "on_hold", "cancelled"}

// Kinds of task. A fix needs a red-green pair (design §6.2).
var Kinds = []string{"feature", "fix", "chore"}

// Meta is the front matter of task.md.
type Meta struct {
	ID            string    `yaml:"id"`
	Title         string    `yaml:"title"`
	Tracker       string    `yaml:"tracker,omitempty"`
	Kind          string    `yaml:"kind"`
	Status        string    `yaml:"status"`
	Class         int       `yaml:"class"`                    // 0 until a human confirms
	ProposedClass int       `yaml:"proposed_class,omitempty"` // proposed by the agent
	Attempt       int       `yaml:"attempt"`
	Delivery      string    `yaml:"delivery,omitempty"` // a task may narrow the delivery mode
	Base          string    `yaml:"base"`               // commit the task's diff is measured from
	Created       time.Time `yaml:"created"`
}

// Task is a loaded task folder.
type Task struct {
	Meta Meta
	Body string
	Dir  string
}

// Paths relative to the repository root.
const (
	TasksDir    = ".leadyard/tasks"
	CurrentFile = ".leadyard/current"
	FileName    = "task.md"
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Dir returns the folder of task id.
func Dir(root, id string) string { return filepath.Join(root, TasksDir, id) }

// New creates a task folder. id may be a tracker key; empty means a new ULID.
func New(root, id, title, kind, tracker, request, base string) (*Task, error) {
	if id == "" {
		id = ledger.NewID()
	}
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("invalid task id %q", id)
	}
	if kind == "" {
		kind = "feature"
	}
	if !contains(Kinds, kind) {
		return nil, fmt.Errorf("unknown kind %q (one of %s)", kind, strings.Join(Kinds, ", "))
	}
	dir := Dir(root, id)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("task %s already exists", id)
	}
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755); err != nil {
		return nil, err
	}
	t := &Task{
		Dir: dir,
		Meta: Meta{ID: id, Title: title, Tracker: tracker, Kind: kind, Status: "new",
			Attempt: 1, Base: base, Created: time.Now().UTC().Truncate(time.Second)},
		Body: template(request),
	}
	return t, t.Save()
}

func template(request string) string {
	if strings.TrimSpace(request) == "" {
		request = "<the original request, copied as is>"
	}
	return "\n## Request (verbatim)\n\n" + strings.TrimSpace(request) + "\n\n" +
		"## Acceptance criteria\n\n<!-- quoted from the ticket or the request: \"- C1. ...\" -->\n\n" +
		"## Plan\n\n<!-- written by the plan step; checkboxes show progress, not acceptance -->\n\n" +
		"## Notes\n\n<!-- working notes for resuming after compaction: hypotheses, running jobs, findings -->\n\n" +
		"## Decisions\n\n<!-- written only by `leadyard decide` and `leadyard transition` -->\n"
}

// Load reads task id.
func Load(root, id string) (*Task, error) {
	dir := Dir(root, id)
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("task %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	meta, body, err := splitFrontMatter(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, FileName), err)
	}
	t := &Task{Dir: dir, Body: body}
	if err := yaml.Unmarshal(meta, &t.Meta); err != nil {
		return nil, fmt.Errorf("%s: front matter: %w", filepath.Join(dir, FileName), err)
	}
	return t, nil
}

func splitFrontMatter(b []byte) ([]byte, string, error) {
	s := string(b)
	if !strings.HasPrefix(s, "---\n") {
		return nil, "", errors.New("missing front matter")
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		return nil, "", errors.New("unterminated front matter")
	}
	return []byte(s[4 : 4+end]), s[4+end+5:], nil
}

// Save writes the front matter and body.
func (t *Task) Save() error {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(t.Meta); err != nil {
		return err
	}
	buf.WriteString("---\n")
	buf.WriteString(t.Body)
	return os.WriteFile(filepath.Join(t.Dir, FileName), buf.Bytes(), 0o644)
}

// AppendToSection adds a line at the end of the "## <name>" section.
func (t *Task) AppendToSection(name, line string) {
	head := "## " + name
	i := strings.Index(t.Body, "\n"+head+"\n")
	if i < 0 {
		t.Body = strings.TrimRight(t.Body, "\n") + "\n\n" + head + "\n\n" + line + "\n"
		return
	}
	start := i + 1 + len(head) + 1
	next := strings.Index(t.Body[start:], "\n## ")
	if next < 0 {
		t.Body = strings.TrimRight(t.Body, "\n") + "\n" + line + "\n"
		return
	}
	pos := start + next
	before := strings.TrimRight(t.Body[:pos], "\n")
	t.Body = before + "\n" + line + "\n" + t.Body[pos:]
}

// Section returns the text of the "## <name>" section without HTML comments.
func (t *Task) Section(name string) string {
	head := "## " + name
	i := strings.Index(t.Body, "\n"+head+"\n")
	if i < 0 {
		if strings.HasPrefix(t.Body, head+"\n") {
			i = -1
		} else {
			return ""
		}
	}
	start := i + 1 + len(head) + 1
	rest := t.Body[start:]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	return strings.TrimSpace(commentRe.ReplaceAllString(rest, ""))
}

var commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

// Criterion is one acceptance criterion.
type Criterion struct {
	ID   string
	Text string
}

var critRe = regexp.MustCompile(`^\s*[-*]\s*(C\d+)[.:)]\s*(.+)$`)

// Criteria parses the acceptance criteria section.
func (t *Task) Criteria() []Criterion {
	var res []Criterion
	for _, l := range strings.Split(t.Section("Acceptance criteria"), "\n") {
		if m := critRe.FindStringSubmatch(l); m != nil {
			res = append(res, Criterion{ID: m[1], Text: strings.TrimSpace(m[2])})
		}
	}
	return res
}

// PlanItem is one checkbox of the plan.
type PlanItem struct {
	Done bool
	Text string
}

var planRe = regexp.MustCompile(`^\s*[-*]\s*\[([ xX])\]\s*(.+)$`)

// Plan parses the plan checkboxes.
func (t *Task) Plan() []PlanItem {
	var res []PlanItem
	for _, l := range strings.Split(t.Section("Plan"), "\n") {
		if m := planRe.FindStringSubmatch(l); m != nil {
			res = append(res, PlanItem{Done: m[1] != " ", Text: strings.TrimSpace(m[2])})
		}
	}
	return res
}

// SetCurrent records the active task of this working tree.
func SetCurrent(root, id string) error {
	return os.WriteFile(filepath.Join(root, CurrentFile), []byte(id+"\n"), 0o644)
}

// Current returns the active task id, or "" when none is set.
func Current(root string) string {
	b, err := os.ReadFile(filepath.Join(root, CurrentFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// List returns the ids of all task folders.
func List(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, TasksDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, TasksDir, e.Name(), FileName)); err == nil {
				ids = append(ids, e.Name())
			}
		}
	}
	return ids, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
