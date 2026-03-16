package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/types"
)

func TestLoad(t *testing.T) {
	t.Run("file not exist returns empty config", func(t *testing.T) {
		cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg == nil || len(cfg.Packages) != 0 || cfg.Agents != nil {
			t.Errorf("expected empty config, got %v", cfg)
		}
	})

	t.Run("valid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "skills.yaml")
		content := "packages:\n  - repo: github.com/foo/bar\n    skills:\n      - my-skill\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		want := &types.SkmConfig{
			Packages: []types.SkillPackageConfig{
				{Repo: "github.com/foo/bar", Skills: []string{"my-skill"}},
			},
		}
		if diff := cmp.Diff(want, cfg); diff != "" {
			t.Errorf("Load() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(path, []byte(":\tinvalid:\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for invalid yaml")
		}
	})
}

func TestSave(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "skills.yaml")
		want := &types.SkmConfig{
			Packages: []types.SkillPackageConfig{
				{Repo: "github.com/a/b", Skills: []string{"s1", "s2"}},
			},
		}
		if err := Save(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("roundtrip mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a", "b", "c", "skills.yaml")
		if err := Save(path, &types.SkmConfig{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file to exist: %v", err)
		}
	})
}

func TestDefaultPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(paths.ConfigFile, home) {
		t.Errorf("ConfigFile %q should have home prefix %q", paths.ConfigFile, home)
	}
	if !strings.Contains(paths.ConfigFile, defaultConfigDir) {
		t.Errorf("ConfigFile %q should contain %q", paths.ConfigFile, defaultConfigDir)
	}
	if !strings.HasPrefix(paths.LockFile, home) {
		t.Errorf("LockFile %q should have home prefix %q", paths.LockFile, home)
	}
	if !strings.HasPrefix(paths.StoreDir, home) {
		t.Errorf("StoreDir %q should have home prefix %q", paths.StoreDir, home)
	}
	if !strings.Contains(paths.StoreDir, defaultStoreBase) {
		t.Errorf("StoreDir %q should contain %q", paths.StoreDir, defaultStoreBase)
	}
}
