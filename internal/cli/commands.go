package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/redikultsev/leadyard/internal/config"
	"github.com/redikultsev/leadyard/internal/initcmd"
	"github.com/redikultsev/leadyard/internal/ledger"
	"github.com/redikultsev/leadyard/internal/level"
	"github.com/redikultsev/leadyard/internal/render"
	"github.com/redikultsev/leadyard/internal/repo"
	"github.com/redikultsev/leadyard/internal/review"
	"github.com/redikultsev/leadyard/internal/task"
	"github.com/redikultsev/leadyard/internal/transition"
)

func cmdInit(e *env, args []string) error {
	exe, _ := os.Executable()
	rep, err := initcmd.Run(e.repo, exe)
	for _, p := range rep.Created {
		fmt.Fprintln(e.out, "created ", p)
	}
	for _, p := range rep.Updated {
		fmt.Fprintln(e.out, "updated ", p)
	}
	for _, p := range rep.Skipped {
		fmt.Fprintln(e.out, "kept    ", p)
	}
	for _, w := range rep.Warnings {
		fmt.Fprintln(e.out, "warning ", w)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(e.out, "\nnext: map your checks to levels in .leadyard/config.yaml, describe zones in .leadyard/zones.yaml, then run `leadyard doctor`.")
	return nil
}

func cmdTask(e *env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: leadyard task new|show|list|use")
	}
	switch args[0] {
	case "new":
		fs := newFlags("task new")
		id := fs.String("id", "", "tracker key; empty for a generated id")
		title := fs.String("title", "", "short title")
		kind := fs.String("kind", "feature", "feature, fix or chore")
		tracker := fs.String("tracker", "", "tracker URL")
		request := fs.String("request", "", "the request, verbatim")
		requestFile := fs.String("request-file", "", "file with the request, verbatim (- for stdin)")
		delivery := fs.String("delivery", "", "narrow the delivery mode for this task")
		if _, err := parse(fs, args[1:]); err != nil {
			return err
		}
		req := *request
		if *requestFile != "" {
			var b []byte
			var err error
			if *requestFile == "-" {
				b, err = io.ReadAll(e.in)
			} else {
				b, err = os.ReadFile(*requestFile)
			}
			if err != nil {
				return err
			}
			req = string(b)
		}
		if *title == "" {
			return errors.New("--title is required")
		}
		if *delivery != "" && !config.AllowsAtLeast(e.cfg.Git.Delivery, *delivery) {
			return fmt.Errorf("--delivery %q is wider than the configured %q", *delivery, e.cfg.Git.Delivery)
		}
		t, err := task.New(e.repo.Root, *id, *title, *kind, *tracker, req, e.repo.BaseCommit(e.cfg.Git.Base))
		if err != nil {
			return err
		}
		if *delivery != "" {
			t.Meta.Delivery = *delivery
			if err := t.Save(); err != nil {
				return err
			}
		}
		if err := task.SetCurrent(e.repo.Root, t.Meta.ID); err != nil {
			return err
		}
		fmt.Fprintf(e.out, "task %s created and set as current: %s\n", t.Meta.ID, filepath.Join(t.Dir, task.FileName))
		if dirty, _ := e.repo.ChangedFiles(t.Meta.Base); len(dirty) > 0 {
			fmt.Fprintf(e.out, "note: %d uncommitted files already differ from the base; they count as part of this task\n", len(dirty))
		}
		return nil
	case "use":
		if len(args) < 2 {
			return errors.New("usage: leadyard task use <id>")
		}
		if _, err := task.Load(e.repo.Root, args[1]); err != nil {
			return err
		}
		return task.SetCurrent(e.repo.Root, args[1])
	case "list":
		ids, err := task.List(e.repo.Root)
		if err != nil {
			return err
		}
		cur := task.Current(e.repo.Root)
		for _, id := range ids {
			t, err := task.Load(e.repo.Root, id)
			if err != nil {
				fmt.Fprintf(e.out, "  %s  (unreadable: %v)\n", id, err)
				continue
			}
			mark := " "
			if id == cur {
				mark = "*"
			}
			fmt.Fprintf(e.out, "%s %s  %-14s class %d  %s\n", mark, id, t.Meta.Status, t.Meta.Class, t.Meta.Title)
		}
		return nil
	case "show":
		fs := newFlags("task show")
		asJSON := fs.Bool("json", false, "")
		pos, err := parse(fs, args[1:])
		if err != nil {
			return err
		}
		id := ""
		if len(pos) > 0 {
			id = pos[0]
		}
		t, _, err := e.current(id)
		if err != nil {
			return err
		}
		if *asJSON {
			return e.json(t.Meta)
		}
		b, err := os.ReadFile(filepath.Join(t.Dir, task.FileName))
		if err != nil {
			return err
		}
		_, err = e.out.Write(b)
		return err
	}
	return fmt.Errorf("unknown task subcommand %q", args[0])
}

func cmdTransition(e *env, args []string) error {
	fs := newFlags("transition")
	byF := fs.String("by", "", `"agent" or "human:<name>"`)
	reason := fs.String("reason", "", "reason; required for some events")
	taskID := fs.String("task", "", "task id (default: current)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: leadyard transition <event> --by agent|human:<name> [--reason ...]")
	}
	if err := by(*byF); err != nil {
		return err
	}
	t, recs, err := e.current(*taskID)
	if err != nil {
		return err
	}
	rep, err := e.report(t, recs)
	if err != nil {
		return err
	}
	in := transition.Input{Event: pos[0], By: *byF, Reason: *reason, Level: rep.Level, Req: rep.Required}
	if err := transition.Apply(t, recs, in); err != nil {
		return err
	}
	fmt.Fprintf(e.out, "task %s: %s (attempt %d)\n", t.Meta.ID, t.Meta.Status, t.Meta.Attempt)
	return nil
}

var decisionKinds = map[string]string{
	"proposed_class":     "agent proposes the risk class: --value 1..3",
	"class":              "human confirms the risk class: --value 1..3",
	"scope_approved":     "human approves the scope",
	"plan_approved":      "human approves the plan",
	"finding_dismissed":  "human dismisses a review finding: --ref <id>",
	"test_edit_accepted": "human accepts an edit of a pre-existing test: --ref <path>",
	"answer":             "an answer to a question: --ref <question> --value <answer>",
	"limit_raised":       "human raises a limit for this task: --ref review_rounds|retries --value N",
	"note":               "anything else worth recording",
}

func cmdDecide(e *env, args []string) error {
	fs := newFlags("decide")
	byF := fs.String("by", "", `"agent" or "human:<name>"`)
	ref := fs.String("ref", "", "what the decision is about")
	value := fs.String("value", "", "the decided value")
	reason := fs.String("reason", "", "why; quote the human's words")
	taskID := fs.String("task", "", "task id (default: current)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		var kinds []string
		for k, v := range decisionKinds {
			kinds = append(kinds, fmt.Sprintf("  %-19s %s", k, v))
		}
		sort.Strings(kinds)
		return fmt.Errorf("usage: leadyard decide <kind> --by ... \nkinds:\n%s", strings.Join(kinds, "\n"))
	}
	kind := pos[0]
	if _, ok := decisionKinds[kind]; !ok {
		return fmt.Errorf("unknown decision kind %q", kind)
	}
	if err := by(*byF); err != nil {
		return err
	}
	human := strings.HasPrefix(*byF, "human:")
	if kind != "proposed_class" && kind != "answer" && kind != "note" && !human {
		return fmt.Errorf("%s is a human decision; pass --by human:<name> after the human said so", kind)
	}
	t, _, err := e.current(*taskID)
	if err != nil {
		return err
	}
	rec := ledger.Record{Type: ledger.TypeDecision, Task: t.Meta.ID, Attempt: t.Meta.Attempt, Kind: kind,
		By: *byF, Ref: *ref, Value: *value, Reason: *reason}
	switch kind {
	case "class", "proposed_class":
		n, err := strconv.Atoi(*value)
		if err != nil || n < 1 || n > 3 {
			return errors.New("--value must be 1, 2 or 3")
		}
		if kind == "class" {
			t.Meta.Class = n
		} else {
			t.Meta.ProposedClass = n
		}
	case "finding_dismissed":
		if *ref == "" {
			return errors.New("--ref <finding id> is required")
		}
	case "test_edit_accepted":
		if *ref == "" {
			return errors.New("--ref <path> is required")
		}
		h, err := e.repo.HashFiles([]string{*ref})
		if err != nil {
			return err
		}
		rec.Value = h[*ref] // binds the acceptance to this exact content
	case "limit_raised":
		if *ref != "review_rounds" && *ref != "retries" {
			return errors.New("--ref must be review_rounds or retries")
		}
	}
	rec, err = ledger.Append(t.Dir, rec)
	if err != nil {
		return err
	}
	t.AppendToSection("Decisions", transition.DecisionLine(rec))
	if err := t.Save(); err != nil {
		return err
	}
	fmt.Fprintln(e.out, "recorded:", transition.DecisionLine(rec))
	return nil
}

func cmdRun(e *env, args []string) error {
	fs := newFlags("run")
	criteria := fs.String("criteria", "", "acceptance criteria this check exercises, e.g. C1,C2")
	taskID := fs.String("task", "", "task id (default: current)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: leadyard run <check-id> [--criteria C1,C2]")
	}
	c, ok := e.cfg.Check(pos[0])
	if !ok {
		var ids []string
		for _, ch := range e.cfg.Checks {
			ids = append(ids, ch.ID)
		}
		return fmt.Errorf("unknown check %q; configured: %s", pos[0], strings.Join(ids, ", "))
	}
	t, _, err := e.current(*taskID)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, c := range t.Criteria() {
		known[c.ID] = true
	}
	var crit []string
	for _, s := range strings.Split(*criteria, ",") {
		if s = strings.TrimSpace(s); s != "" {
			if !known[s] {
				return fmt.Errorf("criterion %s is not in the task's Acceptance criteria; add it as \"- %s. ...\" first", s, s)
			}
			crit = append(crit, s)
		}
	}
	before, err := level.Coverage(e.repo, t.Meta.Base, c)
	if err != nil {
		return err
	}
	id := ledger.NewID()
	logRel := filepath.Join("artifacts", id+".log")
	logFile, err := os.Create(filepath.Join(t.Dir, logRel))
	if err != nil {
		return err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "$ %s\n", c.Run)
	var tail bytes.Buffer
	cmd := exec.Command("sh", "-c", c.Run)
	cmd.Dir = e.repo.Root
	cmd.Stdout = io.MultiWriter(logFile, &tail)
	cmd.Stderr = io.MultiWriter(logFile, &tail)
	start := time.Now()
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			code = ee.ExitCode()
		} else {
			return runErr
		}
	}
	after, err := level.Coverage(e.repo, t.Meta.Base, c)
	if err != nil {
		return err
	}
	status := ledger.Passed
	if code != 0 {
		status = ledger.Failed
	}
	if !sameCov(before, after) {
		status = ledger.Incomplete
	}
	rec, err := ledger.Append(t.Dir, ledger.Record{ID: id, Type: ledger.TypeRun, Task: t.Meta.ID, Attempt: t.Meta.Attempt,
		Check: c.ID, Level: c.Level, Status: status, ExitCode: &code, Artifact: relTo(e.repo.Root, filepath.Join(t.Dir, logRel)),
		Covers: before, CheckDef: level.DefDigest(c), Criteria: crit})
	if err != nil {
		return err
	}
	out := tail.String()
	if len(out) > 3000 {
		out = "…\n" + out[len(out)-3000:]
	}
	fmt.Fprint(e.out, out)
	fmt.Fprintf(e.out, "\n%s: %s (exit %d, %s) · record %s · log %s\n", c.ID, status, code,
		time.Since(start).Round(time.Millisecond), rec.ID, rec.Artifact)
	if status == ledger.Incomplete {
		fmt.Fprintln(e.out, "files changed while the check ran; the result does not count. Run it again.")
	}
	return nil
}

func sameCov(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func cmdLevel(e *env, args []string) error {
	fs := newFlags("level")
	asJSON := fs.Bool("json", false, "")
	taskID := fs.String("task", "", "task id (default: current)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	t, recs, err := e.current(*taskID)
	if err != nil {
		return err
	}
	rep, err := e.report(t, recs)
	if err != nil {
		return err
	}
	if *asJSON {
		return e.json(rep)
	}
	fmt.Fprintf(e.out, "task %s — %s · status %s · attempt %d\n", t.Meta.ID, t.Meta.Title, t.Meta.Status, t.Meta.Attempt)
	fmt.Fprint(e.out, render.Level(rep))
	return nil
}

func cmdReview(e *env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: leadyard review input | review record --file <answer>")
	}
	fs := newFlags("review")
	file := fs.String("file", "", "file with the reviewer's full answer (- for stdin)")
	taskID := fs.String("task", "", "task id (default: current)")
	if _, err := parse(fs, args[1:]); err != nil {
		return err
	}
	t, recs, err := e.current(*taskID)
	if err != nil {
		return err
	}
	switch args[0] {
	case "input":
		s, err := review.Input(e.repo, t, recs)
		if err != nil {
			return err
		}
		_, err = io.WriteString(e.out, s)
		return err
	case "record":
		rounds := 0
		limit := e.cfg.Limits.ReviewRounds
		for _, r := range recs {
			if r.Type == ledger.TypeReview && r.Attempt == t.Meta.Attempt {
				rounds++
			}
			if r.Type == ledger.TypeDecision && r.Kind == "limit_raised" && r.Ref == "review_rounds" {
				if n, err := strconv.Atoi(r.Value); err == nil && n > limit {
					limit = n
				}
			}
		}
		var b []byte
		switch *file {
		case "":
			return errors.New("--file is required")
		case "-":
			b, err = io.ReadAll(e.in)
		default:
			b, err = os.ReadFile(*file)
		}
		if err != nil {
			return err
		}
		v, err := review.Parse(string(b))
		if err != nil {
			return err
		}
		rec, err := review.Record(e.repo, t, recs, v)
		if err != nil {
			return err
		}
		blocking := 0
		for _, f := range rec.Findings {
			if f.Severity == "blocking" {
				blocking++
			}
		}
		fmt.Fprintf(e.out, "review recorded (round %d of %d): status %s, %d open findings, %d blocking\n",
			rounds+1, limit, v.Status, len(rec.Findings), blocking)
		for _, f := range rec.Findings {
			fmt.Fprintf(e.out, "- %s [%s] %s:%d %s\n", f.ID, f.Severity, f.File, f.Line, f.Summary)
		}
		if rounds+1 >= limit && blocking > 0 {
			fmt.Fprintln(e.out, "\nreview round limit reached: hand the task to the human (`leadyard transition ask_human --by agent --reason \"review limit\"`).")
		}
		return nil
	}
	return fmt.Errorf("unknown review subcommand %q", args[0])
}

var manifests = []string{"go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
	"pyproject.toml", "uv.lock", "poetry.lock", "Pipfile", "Pipfile.lock", "Cargo.toml", "Cargo.lock",
	"pom.xml", "build.gradle", "build.gradle.kts", "Gemfile", "Gemfile.lock", "composer.json"}

func cmdScope(e *env, args []string) error {
	fs := newFlags("scope")
	asJSON := fs.Bool("json", false, "")
	taskID := fs.String("task", "", "task id (default: current)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	t, _, err := e.current(*taskID)
	if err != nil {
		return err
	}
	changed, err := e.repo.ChangedFiles(t.Meta.Base)
	if err != nil {
		return err
	}
	plan := t.Section("Plan")
	type result struct {
		Changed     []string `json:"changed"`
		NotInPlan   []string `json:"not_in_plan"`
		Protected   []string `json:"protected_paths"`
		Manifests   []string `json:"dependency_manifests"`
		Zones       []string `json:"zones"`
		ZoneClass   int      `json:"zone_class"`
		TaskClass   int      `json:"task_class"`
		ClassRaised bool     `json:"class_raised"`
	}
	res := result{Changed: changed, TaskClass: t.Meta.Class}
	for _, f := range changed {
		if !strings.Contains(plan, f) {
			res.NotInPlan = append(res.NotInPlan, f)
		}
		if repo.MatchAny(e.cfg.Git.ProtectedPaths, f) {
			res.Protected = append(res.Protected, f)
		}
		for _, m := range manifests {
			if filepath.Base(f) == m {
				res.Manifests = append(res.Manifests, f)
			}
		}
	}
	res.Zones, res.ZoneClass = zonesFor(e.cfg, changed)
	res.ClassRaised = res.ZoneClass > t.Meta.Class
	if *asJSON {
		return e.json(res)
	}
	fmt.Fprintf(e.out, "changed files: %d\n", len(changed))
	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintln(e.out, "\n"+title+":")
		for _, s := range items {
			fmt.Fprintln(e.out, "- "+s)
		}
	}
	list("not mentioned in the plan (remove, or keep with a reason)", res.NotInPlan)
	list("protected paths", res.Protected)
	list("dependency manifests", res.Manifests)
	list("zones touched", res.Zones)
	if res.ClassRaised {
		fmt.Fprintf(e.out, "\nzones raise the class to %d (confirmed: %d); the human must confirm the new class.\n", res.ZoneClass, t.Meta.Class)
	}
	return nil
}

func zonesFor(cfg config.Config, files []string) ([]string, int) {
	var hit []string
	max := 0
	for _, z := range cfg.Zones {
		for _, f := range files {
			if repo.MatchAny(z.Paths, f) {
				hit = append(hit, fmt.Sprintf("%s (class %d): %s", z.ID, z.Class, f))
				if z.Class > max {
					max = z.Class
				}
				break
			}
		}
	}
	return hit, max
}

func cmdZones(e *env, args []string) error {
	fs := newFlags("zones")
	taskID := fs.String("task", "", "task id (default: current)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	base := e.repo.BaseCommit(e.cfg.Git.Base)
	if t, _, err := e.current(*taskID); err == nil {
		base = t.Meta.Base
	}
	changed, err := e.repo.ChangedFiles(base)
	if err != nil {
		return err
	}
	hit, max := zonesFor(e.cfg, changed)
	if len(e.cfg.Zones) == 0 {
		fmt.Fprintln(e.out, "no zones configured (.leadyard/zones.yaml)")
		return nil
	}
	for _, h := range hit {
		fmt.Fprintln(e.out, "- "+h)
	}
	fmt.Fprintf(e.out, "zone class: %d\n", max)
	return nil
}

func cmdResume(e *env, args []string) error {
	fs := newFlags("resume")
	quiet := fs.Bool("quiet", false, "print nothing when there is no current task")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	t, recs, err := e.current("")
	if err != nil {
		if *quiet {
			return nil
		}
		return err
	}
	rep, err := e.report(t, recs)
	if err != nil {
		return err
	}
	_, err = io.WriteString(e.out, render.Resume(t, rep, recs, e.delivery(t)))
	return err
}

func cmdPRBody(e *env, args []string) error {
	t, recs, err := e.current("")
	if err != nil {
		return err
	}
	rep, err := e.report(t, recs)
	if err != nil {
		return err
	}
	_, err = io.WriteString(e.out, render.PRBody(t, rep))
	return err
}

func cmdConfig(e *env, args []string) error {
	b, err := yaml.Marshal(e.cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.out, "# effective configuration (team %s, user %s)\n", config.TeamFile, config.UserFile())
	_, err = e.out.Write(b)
	if len(e.cfg.Zones) > 0 {
		zb, _ := yaml.Marshal(map[string]any{"zones": e.cfg.Zones})
		e.out.Write(zb)
	}
	return err
}

func relTo(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}
