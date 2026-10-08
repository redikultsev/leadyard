package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLayers(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	write(t, filepath.Join(root, TeamFile), `schema_version: 1
git:
  delivery: pr
  base: develop
checks:
  - {id: tests, level: 3, run: "go test ./...", tests: ["**/*_test.go"]}
`)
	write(t, filepath.Join(root, ZonesFile), `zones:
  - {id: prompts, class: 3, paths: ["prompts/**"]}
`)
	write(t, filepath.Join(home, "leadyard", "config.yaml"), "git:\n  delivery: none\n")
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Git.Delivery != DeliveryNone {
		t.Errorf("user layer must tighten delivery to none, got %q", cfg.Git.Delivery)
	}
	if cfg.Git.Base != "develop" || len(cfg.Checks) != 1 || len(cfg.Zones) != 1 {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.ClassLevels[2] != 3 {
		t.Errorf("default class levels lost: %v", cfg.ClassLevels)
	}
}

func TestUserLayerCannotWiden(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	write(t, filepath.Join(root, TeamFile), "schema_version: 1\ngit:\n  delivery: local\n")
	write(t, filepath.Join(home, "leadyard", "config.yaml"), "git:\n  delivery: pr\n")
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Git.Delivery != DeliveryLocal {
		t.Errorf("user layer widened delivery to %q", cfg.Git.Delivery)
	}
}

func TestValidate(t *testing.T) {
	c := Default()
	c.Checks = []Check{{ID: "review", Level: 3, Run: "x"}}
	if c.Validate() == nil {
		t.Error("reserved id accepted")
	}
	c.Checks = []Check{{ID: "a", Level: 5, Run: "x"}}
	if c.Validate() == nil {
		t.Error("level 5 check accepted")
	}
}
