// Package repo wraps the git queries leadyard needs: the repository root, the
// files of the task's diff against a base, and content hashes of files.
package repo

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Repo is a git working tree.
type Repo struct {
	Root string
}

// Open finds the repository that contains dir.
func Open(dir string) (*Repo, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	return &Repo{Root: strings.TrimSpace(out)}, nil
}

// Git runs a git command in the repository root and returns stdout.
func (r *Repo) Git(args ...string) (string, error) {
	return git(r.Root, args...)
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// CurrentBranch returns the checked-out branch, or "" when HEAD is detached
// or the repository has no commits yet.
func (r *Repo) CurrentBranch() string {
	out, err := r.Git("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// MergeBase returns the merge base of HEAD and base, or "" when it cannot be
// found (no commits, unknown base).
func (r *Repo) MergeBase(base string) string {
	out, err := r.Git("merge-base", "HEAD", base)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// excluded reports whether a path belongs to leadyard's own state and must not
// take part in digests or diffs.
func excluded(path string) bool {
	return path == ".leadyard" || strings.HasPrefix(path, ".leadyard/")
}

// ChangedFiles lists files that differ between the commit rev and the working
// tree, including untracked files that are not ignored. Paths under .leadyard/
// are skipped. With an empty rev every working file counts as changed.
func (r *Repo) ChangedFiles(rev string) ([]string, error) {
	set := map[string]bool{}
	if rev != "" {
		out, err := r.Git("diff", "--name-only", rev)
		if err != nil {
			return nil, err
		}
		addLines(set, out)
		out, err = r.Git("ls-files", "--others", "--exclude-standard")
		if err != nil {
			return nil, err
		}
		addLines(set, out)
		return sortedKeys(set), nil
	}
	return r.WorkingFiles()
}

// Head returns the commit of HEAD, or "" in a repository without commits.
func (r *Repo) Head() string {
	out, err := r.Git("rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// BaseCommit returns the merge base of HEAD with branch, falling back to HEAD.
func (r *Repo) BaseCommit(branch string) string {
	if mb := r.MergeBase(branch); mb != "" {
		return mb
	}
	return r.Head()
}

// FilesAt lists the files that existed at commit rev.
func (r *Repo) FilesAt(rev string) (map[string]bool, error) {
	set := map[string]bool{}
	if rev == "" {
		return set, nil
	}
	out, err := r.Git("ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	addLines(set, out)
	return set, nil
}

// WorkingFiles lists tracked and untracked (not ignored) files of the working tree.
func (r *Repo) WorkingFiles() ([]string, error) {
	set := map[string]bool{}
	out, err := r.Git("ls-files", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	addLines(set, out)
	return sortedKeys(set), nil
}

// HashFiles returns git blob hashes of the given working-tree files. A file that
// does not exist maps to "deleted".
func (r *Repo) HashFiles(paths []string) (map[string]string, error) {
	res := make(map[string]string, len(paths))
	var present []string
	for _, p := range paths {
		if fileExists(filepath.Join(r.Root, p)) {
			present = append(present, p)
		} else {
			res[p] = "deleted"
		}
	}
	if len(present) == 0 {
		return res, nil
	}
	cmd := exec.Command("git", "hash-object", "--stdin-paths")
	cmd.Dir = r.Root
	cmd.Stdin = strings.NewReader(strings.Join(present, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git hash-object: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	hashes := strings.Fields(out.String())
	if len(hashes) != len(present) {
		return nil, fmt.Errorf("git hash-object: got %d hashes for %d files", len(hashes), len(present))
	}
	for i, p := range present {
		res[p] = hashes[i]
	}
	return res, nil
}

func addLines(set map[string]bool, out string) {
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !excluded(l) {
			set[l] = true
		}
	}
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
