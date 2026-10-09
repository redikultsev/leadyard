// Package level computes the evidence level of a task from its ledger
// (design §5.4): cumulative over the latest fresh record of each check, with the
// level-3 conditions (review without open blocking findings, accepted edits of
// pre-existing tests).
package level

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/redikultsev/leadyard/internal/config"
	"github.com/redikultsev/leadyard/internal/ledger"
	"github.com/redikultsev/leadyard/internal/repo"
	"github.com/redikultsev/leadyard/internal/task"
)

// Check states shown to people.
const (
	Missing = "missing"
	Stale   = "stale"
)

// CheckState is the state of one configured check for the current attempt.
type CheckState struct {
	ID       string   `json:"id"`
	Level    int      `json:"level"`
	Status   string   `json:"status"` // passed, failed, incomplete, missing, stale
	Record   string   `json:"record,omitempty"`
	Artifact string   `json:"artifact,omitempty"`
	Criteria []string `json:"criteria,omitempty"`
}

// CriterionState links an acceptance criterion to the checks that verify it.
type CriterionState struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Checks   []string `json:"checks,omitempty"`
	Verified bool     `json:"verified"`
}

// Report is the result of a level computation.
type Report struct {
	Task           string           `json:"task"`
	Attempt        int              `json:"attempt"`
	Class          int              `json:"class"`
	Required       int              `json:"required"`
	Level          int              `json:"level"`
	Below          bool             `json:"below_requirement"`
	Checks         []CheckState     `json:"checks"`
	ReviewStatus   string           `json:"review"` // missing, stale, done
	OpenBlocking   []ledger.Finding `json:"open_blocking,omitempty"`
	OpenAdvisory   []ledger.Finding `json:"open_advisory,omitempty"`
	EditedTests    []string         `json:"edited_tests,omitempty"`
	UnacceptedEdit []string         `json:"unaccepted_test_edits,omitempty"`
	Criteria       []CriterionState `json:"criteria,omitempty"`
	NotVerified    []string         `json:"not_verified,omitempty"`
}

// DefDigest fingerprints a check definition so that a changed command makes
// earlier records stale.
func DefDigest(c config.Check) string {
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// Coverage returns the files a check depends on: the task's changed files plus
// working files matching the check's declared inputs, with their hashes.
func Coverage(r *repo.Repo, base string, c config.Check) (map[string]string, error) {
	changed, err := r.ChangedFiles(base)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, f := range changed {
		set[f] = true
	}
	if len(c.Inputs) > 0 {
		files, err := r.WorkingFiles()
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if repo.MatchAny(c.Inputs, f) {
				set[f] = true
			}
		}
	}
	paths := make([]string, 0, len(set))
	for f := range set {
		paths = append(paths, f)
	}
	sort.Strings(paths)
	return r.HashFiles(paths)
}

func sameMap(a, b map[string]string) bool {
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

// Compute builds the report for task t.
func Compute(r *repo.Repo, cfg config.Config, t *task.Task, recs []ledger.Record) (Report, error) {
	m := t.Meta
	rep := Report{Task: m.ID, Attempt: m.Attempt, Class: m.Class}
	if m.Class > 0 {
		rep.Required = cfg.ClassLevels[m.Class]
	}

	dismissed := map[string]bool{}
	accepted := map[string]string{} // path -> accepted hash
	for _, rec := range recs {
		if rec.Type != ledger.TypeDecision {
			continue
		}
		switch rec.Kind {
		case "finding_dismissed":
			dismissed[rec.Ref] = true
		case "test_edit_accepted":
			accepted[rec.Ref] = rec.Value
		}
	}

	// Latest run record per check in the current attempt.
	latest := map[string]ledger.Record{}
	var lastReview *ledger.Record
	for i := range recs {
		rec := recs[i]
		if rec.Attempt != m.Attempt {
			continue
		}
		switch rec.Type {
		case ledger.TypeRun:
			latest[rec.Check] = rec
		case ledger.TypeReview:
			lastReview = &recs[i]
		}
	}

	passedFresh := map[string]CheckState{}
	for _, c := range cfg.Checks {
		st := CheckState{ID: c.ID, Level: c.Level, Status: Missing}
		if rec, ok := latest[c.ID]; ok {
			st.Record, st.Artifact, st.Criteria = rec.ID, rec.Artifact, rec.Criteria
			cov, err := Coverage(r, m.Base, c)
			if err != nil {
				return rep, err
			}
			if rec.CheckDef != DefDigest(c) || !sameMap(rec.Covers, cov) {
				st.Status = Stale
			} else {
				st.Status = rec.Status
			}
		}
		if st.Status == ledger.Passed {
			passedFresh[c.ID] = st
		}
		rep.Checks = append(rep.Checks, st)
	}

	// Cumulative level: highest level that has a passed check, with every check
	// at or below it passed.
	for l := 1; l <= 4; l++ {
		any, all := false, true
		for _, c := range cfg.Checks {
			if c.Level > l {
				continue
			}
			if _, ok := passedFresh[c.ID]; ok {
				if c.Level == l {
					any = true
				}
			} else {
				all = false
			}
		}
		if !all {
			break
		}
		if any {
			rep.Level = l
		}
	}

	// Review state.
	rep.ReviewStatus = Missing
	if lastReview != nil {
		changed, err := r.ChangedFiles(m.Base)
		if err != nil {
			return rep, err
		}
		cov, err := r.HashFiles(changed)
		if err != nil {
			return rep, err
		}
		if sameMap(lastReview.Covers, cov) {
			rep.ReviewStatus = "done"
		} else {
			rep.ReviewStatus = Stale
		}
		for _, f := range lastReview.Findings {
			if dismissed[f.ID] {
				continue
			}
			if f.Severity == "blocking" {
				rep.OpenBlocking = append(rep.OpenBlocking, f)
			} else {
				rep.OpenAdvisory = append(rep.OpenAdvisory, f)
			}
		}
	}

	// Edited pre-existing test files.
	var testGlobs []string
	for _, c := range cfg.Checks {
		testGlobs = append(testGlobs, c.Tests...)
	}
	if len(testGlobs) > 0 {
		changed, err := r.ChangedFiles(m.Base)
		if err != nil {
			return rep, err
		}
		atBase, err := r.FilesAt(m.Base)
		if err != nil {
			return rep, err
		}
		var edited []string
		for _, f := range changed {
			if atBase[f] && repo.MatchAny(testGlobs, f) {
				edited = append(edited, f)
			}
		}
		hashes, err := r.HashFiles(edited)
		if err != nil {
			return rep, err
		}
		for _, f := range edited {
			rep.EditedTests = append(rep.EditedTests, f)
			if accepted[f] != hashes[f] {
				rep.UnacceptedEdit = append(rep.UnacceptedEdit, f)
			}
		}
	}

	// Level-3 conditions.
	if rep.Level >= 3 && (rep.ReviewStatus != "done" || len(rep.OpenBlocking) > 0 || len(rep.UnacceptedEdit) > 0) {
		rep.Level = 2
	}

	// Acceptance criteria.
	for _, c := range t.Criteria() {
		cs := CriterionState{ID: c.ID, Text: c.Text}
		for _, st := range rep.Checks {
			for _, id := range st.Criteria {
				if id == c.ID {
					cs.Checks = append(cs.Checks, st.ID)
					if st.Status == ledger.Passed {
						cs.Verified = true
					}
				}
			}
		}
		if !cs.Verified {
			rep.NotVerified = append(rep.NotVerified, c.ID+" "+c.Text)
		}
		rep.Criteria = append(rep.Criteria, cs)
	}
	for _, st := range rep.Checks {
		if st.Status != ledger.Passed {
			rep.NotVerified = append(rep.NotVerified, "check "+st.ID+": "+st.Status)
		}
	}
	if rep.ReviewStatus != "done" {
		rep.NotVerified = append(rep.NotVerified, "review: "+rep.ReviewStatus)
	}
	rep.Below = rep.Required == 0 || rep.Level < rep.Required
	return rep, nil
}
