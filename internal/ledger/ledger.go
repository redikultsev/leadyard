// Package ledger reads and appends the task's records: check runs, reviews,
// links from acceptance criteria to checks, and decisions (design §5.4, §5.7).
// The file is append-only JSON lines; records are never rewritten.
package ledger

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Record types.
const (
	TypeRun      = "run"
	TypeReview   = "review"
	TypeLink     = "link"
	TypeDecision = "decision"
)

// Record statuses (design §5.4).
const (
	Passed     = "passed"
	Failed     = "failed"
	Incomplete = "incomplete"
)

// Finding is one review finding (design §5.8.1).
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Summary  string `json:"summary"`
}

// Record is one line of the ledger. Fields are filled according to Type.
type Record struct {
	V          int               `json:"v"`
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	Task       string            `json:"task"`
	Attempt    int               `json:"attempt"`
	RecordedAt time.Time         `json:"recorded_at"`
	Check      string            `json:"check,omitempty"`
	Level      int               `json:"level,omitempty"`
	Status     string            `json:"status,omitempty"`
	ExitCode   *int              `json:"exit_code,omitempty"`
	Artifact   string            `json:"artifact,omitempty"`
	Covers     map[string]string `json:"covers,omitempty"`
	CheckDef   string            `json:"check_def,omitempty"` // digest of the check definition used
	Criteria   []string          `json:"criteria,omitempty"`  // acceptance criteria this run verifies
	Findings   []Finding         `json:"findings,omitempty"`
	Criterion  string            `json:"criterion,omitempty"`
	Kind       string            `json:"kind,omitempty"`  // decision kind
	By         string            `json:"by,omitempty"`    // who decided
	Ref        string            `json:"ref,omitempty"`   // what the decision is about
	Value      string            `json:"value,omitempty"` // decision value
	Reason     string            `json:"reason,omitempty"`
}

// File is the ledger file name inside a task folder.
const File = "evidence.jsonl"

// Append writes rec to the ledger in dir, filling V, ID and RecordedAt.
func Append(dir string, rec Record) (Record, error) {
	rec.V = 1
	if rec.ID == "" {
		rec.ID = NewID()
	}
	if rec.RecordedAt.IsZero() {
		rec.RecordedAt = time.Now().UTC()
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return rec, err
	}
	f, err := os.OpenFile(filepath.Join(dir, File), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return rec, err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return rec, err
	}
	return rec, nil
}

// Read returns all records in file order. A missing ledger is empty.
func Read(dir string) ([]Record, error) {
	f, err := os.Open(filepath.Join(dir, File))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var recs []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if s == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(s), &r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", File, line, err)
		}
		recs = append(recs, r)
	}
	return recs, sc.Err()
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID returns a ULID: 48 bits of milliseconds and 80 random bits, base32.
// ULIDs sort by time and do not collide across branches.
func NewID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err)
	}
	// 128 bits -> 26 base32 chars (first char carries 3 bits).
	var out [26]byte
	var acc uint64
	var bits uint
	idx := 25
	for i := 15; i >= 0; i-- {
		acc |= uint64(b[i]) << bits
		bits += 8
		for bits >= 5 && idx >= 0 {
			out[idx] = crockford[acc&31]
			acc >>= 5
			bits -= 5
			idx--
		}
	}
	for idx >= 0 {
		out[idx] = crockford[acc&31]
		acc >>= 5
		idx--
	}
	return string(out[:])
}
