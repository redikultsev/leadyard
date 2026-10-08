// Package config loads leadyard configuration: the team file in the repository,
// the user file shared by all repositories of one person, and the zones file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Delivery modes, from the most restrictive to the least (design §7.2).
const (
	DeliveryNone   = "none"
	DeliveryLocal  = "local"
	DeliveryBranch = "branch"
	DeliveryPR     = "pr"
)

var deliveryRank = map[string]int{DeliveryNone: 0, DeliveryLocal: 1, DeliveryBranch: 2, DeliveryPR: 3}

// Check is a project command mapped to an evidence level (design §5.4).
type Check struct {
	ID     string   `yaml:"id"`
	Level  int      `yaml:"level"`
	Run    string   `yaml:"run"`
	Inputs []string `yaml:"inputs,omitempty"` // globs the check depends on besides the task diff
	Tests  []string `yaml:"tests,omitempty"`  // globs of test files; edits to pre-existing ones need acceptance
}

// Git is the git policy of the project (design §7).
type Git struct {
	Delivery          string   `yaml:"delivery"`
	Base              string   `yaml:"base"`
	ProtectedBranches []string `yaml:"protected_branches"`
	ProtectedPaths    []string `yaml:"protected_paths"`
	BranchPattern     string   `yaml:"branch_pattern"`
}

// Config is the merged configuration.
type Config struct {
	SchemaVersion int         `yaml:"schema_version"`
	Agents        []string    `yaml:"agents"`
	Git           Git         `yaml:"git"`
	Checks        []Check     `yaml:"checks"`
	ClassLevels   map[int]int `yaml:"class_levels"`
	Limits        Limits      `yaml:"limits"`
	Zones         []Zone      `yaml:"-"`
}

// Limits bound automatic loops (design §5.8).
type Limits struct {
	ReviewRounds int `yaml:"review_rounds"`
	Retries      int `yaml:"retries"`
}

// Zone is an area where a change is sensitive (design §5.5).
type Zone struct {
	ID    string   `yaml:"id"`
	Class int      `yaml:"class"`
	Why   string   `yaml:"why"`
	Paths []string `yaml:"paths"`
}

type zonesFile struct {
	Zones []Zone `yaml:"zones"`
}

// Paths of configuration files relative to the repository root.
const (
	Dir       = ".leadyard"
	TeamFile  = ".leadyard/config.yaml"
	ZonesFile = ".leadyard/zones.yaml"
)

// Default returns the distribution defaults.
func Default() Config {
	return Config{
		SchemaVersion: 1,
		Agents:        []string{"claude-code"},
		Git: Git{
			Delivery:          DeliveryPR,
			Base:              "main",
			ProtectedBranches: []string{"main", "master", "develop", "release/*"},
			ProtectedPaths:    []string{".github/workflows/**", "CODEOWNERS", ".leadyard/config.yaml", ".leadyard/zones.yaml"},
			BranchPattern:     "agent/{task}",
		},
		ClassLevels: map[int]int{1: 1, 2: 3, 3: 4},
		Limits:      Limits{ReviewRounds: 5, Retries: 5},
	}
}

// UserFile returns the path of the user-level config.
func UserFile() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "leadyard", "config.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "leadyard", "config.yaml")
}

// Load reads the configuration of the repository at root: defaults, then the team
// file, then the user file, which may only tighten the git delivery mode.
func Load(root string) (Config, error) {
	cfg := Default()
	if err := mergeFile(&cfg, filepath.Join(root, TeamFile)); err != nil {
		return cfg, err
	}
	if uf := UserFile(); uf != "" {
		var user Config
		if err := readYAML(uf, &user); err != nil && !errors.Is(err, os.ErrNotExist) {
			return cfg, err
		}
		if user.Git.Delivery != "" {
			if _, ok := deliveryRank[user.Git.Delivery]; !ok {
				return cfg, fmt.Errorf("%s: unknown git.delivery %q", uf, user.Git.Delivery)
			}
			if deliveryRank[user.Git.Delivery] < deliveryRank[cfg.Git.Delivery] {
				cfg.Git.Delivery = user.Git.Delivery
			}
		}
	}
	var zf zonesFile
	if err := readYAML(filepath.Join(root, ZonesFile), &zf); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	cfg.Zones = zf.Zones
	return cfg, cfg.Validate()
}

func mergeFile(cfg *Config, path string) error {
	var team Config
	err := readYAML(path, &team)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if team.SchemaVersion != 0 {
		cfg.SchemaVersion = team.SchemaVersion
	}
	if len(team.Agents) > 0 {
		cfg.Agents = team.Agents
	}
	g := team.Git
	if g.Delivery != "" {
		cfg.Git.Delivery = g.Delivery
	}
	if g.Base != "" {
		cfg.Git.Base = g.Base
	}
	if len(g.ProtectedBranches) > 0 {
		cfg.Git.ProtectedBranches = g.ProtectedBranches
	}
	if len(g.ProtectedPaths) > 0 {
		cfg.Git.ProtectedPaths = g.ProtectedPaths
	}
	if g.BranchPattern != "" {
		cfg.Git.BranchPattern = g.BranchPattern
	}
	if len(team.Checks) > 0 {
		cfg.Checks = team.Checks
	}
	for k, v := range team.ClassLevels {
		cfg.ClassLevels[k] = v
	}
	if team.Limits.ReviewRounds > 0 {
		cfg.Limits.ReviewRounds = team.Limits.ReviewRounds
	}
	if team.Limits.Retries > 0 {
		cfg.Limits.Retries = team.Limits.Retries
	}
	return nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Validate checks values the rest of the program relies on.
func (c Config) Validate() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d (this leadyard understands 1)", c.SchemaVersion)
	}
	if _, ok := deliveryRank[c.Git.Delivery]; !ok {
		return fmt.Errorf("unknown git.delivery %q", c.Git.Delivery)
	}
	seen := map[string]bool{}
	for _, ch := range c.Checks {
		if ch.ID == "" || ch.Run == "" {
			return fmt.Errorf("check needs id and run: %+v", ch)
		}
		if ch.ID == "review" {
			return fmt.Errorf("check id %q is reserved", ch.ID)
		}
		if seen[ch.ID] {
			return fmt.Errorf("duplicate check id %q", ch.ID)
		}
		seen[ch.ID] = true
		if ch.Level < 1 || ch.Level > 4 {
			return fmt.Errorf("check %q: level must be 1..4", ch.ID)
		}
	}
	for _, z := range c.Zones {
		if z.Class < 1 || z.Class > 3 {
			return fmt.Errorf("zone %q: class must be 1..3", z.ID)
		}
	}
	return nil
}

// Check returns the check with the given id.
func (c Config) Check(id string) (Check, bool) {
	for _, ch := range c.Checks {
		if ch.ID == id {
			return ch, true
		}
	}
	return Check{}, false
}

// AllowsAtLeast reports whether the delivery mode allows everything mode m allows.
func AllowsAtLeast(delivery, m string) bool {
	return deliveryRank[delivery] >= deliveryRank[m]
}
