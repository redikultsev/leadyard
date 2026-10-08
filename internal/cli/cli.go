// Package cli implements the leadyard commands.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zireaelq/leadyard/internal/config"
	"github.com/zireaelq/leadyard/internal/ledger"
	"github.com/zireaelq/leadyard/internal/level"
	"github.com/zireaelq/leadyard/internal/repo"
	"github.com/zireaelq/leadyard/internal/task"
)

// Version is set at build time.
var Version = "0.1.0-dev"

const usage = `leadyard — evidence-based developer loop for coding agents

usage: leadyard <command> [flags]

  init                       install into this repository (Claude Code adapter, git hooks)
  doctor                     check the installation; gate self-test
  task new|show|list|use     manage task folders
  transition <event>         change the task status (the only way)
  decide <kind>              record a decision
  run <check>                run a configured check and record the result
  level                      evidence level and verdict package
  review input|record        build the reviewer bundle; record its verdict
  scope                      deterministic scope audit of the final diff
  zones                      zones touched by the diff and the resulting class
  resume                     task state for a new or compacted session
  pr-body                    draft pull request description
  config                     print the effective configuration
  gate                       hook entry point (reads a tool call on stdin)
  githook pre-push|commit-msg  git hook entry points
  version

Add --json to most commands for machine output.
`

// env is what a command needs.
type env struct {
	in       io.Reader
	out, err io.Writer
	repo     *repo.Repo
	cfg      config.Config
}

// Main dispatches a command and returns the exit code.
func Main(args []string, in io.Reader, out, errw io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, usage)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Fprintln(out, "leadyard", Version)
		return 0
	}
	cmd, rest := args[0], args[1:]
	// The gate must fail closed on any error, including outside a repository.
	if cmd == "gate" {
		return runGate(in, out, errw)
	}
	if cmd == "githook" {
		return runGitHook(rest, in, out, errw)
	}
	e := &env{in: in, out: out, err: errw}
	wd, _ := os.Getwd()
	r, err := repo.Open(wd)
	if err != nil {
		fmt.Fprintln(errw, "leadyard:", err)
		return 1
	}
	e.repo = r
	if cmd != "init" {
		cfg, err := config.Load(r.Root)
		if err != nil {
			fmt.Fprintln(errw, "leadyard: config:", err)
			return 1
		}
		e.cfg = cfg
	}
	commands := map[string]func(*env, []string) error{
		"init": cmdInit, "doctor": cmdDoctor, "task": cmdTask, "transition": cmdTransition,
		"decide": cmdDecide, "run": cmdRun, "level": cmdLevel, "review": cmdReview, "scope": cmdScope,
		"zones": cmdZones, "resume": cmdResume, "pr-body": cmdPRBody, "config": cmdConfig,
	}
	fn, ok := commands[cmd]
	if !ok {
		fmt.Fprintf(errw, "leadyard: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	if err := fn(e, rest); err != nil {
		fmt.Fprintln(errw, "leadyard:", err)
		var ec exitCode
		if errors.As(err, &ec) {
			return int(ec)
		}
		return 1
	}
	return 0
}

type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit %d", int(c)) }

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parse lets flags follow positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// current loads the active task, or the one named by --task.
func (e *env) current(id string) (*task.Task, []ledger.Record, error) {
	if id == "" {
		id = task.Current(e.repo.Root)
	}
	if id == "" {
		return nil, nil, errors.New("no current task; run `leadyard task new` or `leadyard task use <id>`")
	}
	t, err := task.Load(e.repo.Root, id)
	if err != nil {
		return nil, nil, err
	}
	recs, err := ledger.Read(t.Dir)
	if err != nil {
		return nil, nil, err
	}
	return t, recs, nil
}

func (e *env) report(t *task.Task, recs []ledger.Record) (level.Report, error) {
	return level.Compute(e.repo, e.cfg, t, recs)
}

// delivery is the effective delivery mode for a task: the task may narrow it.
func (e *env) delivery(t *task.Task) string {
	d := e.cfg.Git.Delivery
	if t != nil && t.Meta.Delivery != "" && config.AllowsAtLeast(d, t.Meta.Delivery) {
		d = t.Meta.Delivery
	}
	return d
}

func (e *env) json(v any) error {
	enc := json.NewEncoder(e.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func by(s string) error {
	if s != "agent" && !strings.HasPrefix(s, "human:") {
		return errors.New(`--by must be "agent" or "human:<name>"`)
	}
	return nil
}
