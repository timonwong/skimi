package lock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/types"
)

func TestLoad(t *testing.T) {
	t.Run("file not exist returns empty", func(t *testing.T) {
		lf, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if lf == nil || len(lf.Skills) != 0 {
			t.Errorf("expected empty lock file, got %v", lf)
		}
	})

	t.Run("valid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lock.yaml")
		content := "skills:\n  - name: my-skill\n    repo: github.com/foo/bar\n    commit: abc123\n    skill_path: /store/my-skill\n    linked_to:\n      - /home/user/.claude/skills/my-skill\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		lf, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		want := &types.LockFile{
			Skills: []types.InstalledSkill{
				{
					Name:      "my-skill",
					Repo:      "github.com/foo/bar",
					Commit:    "abc123",
					SkillPath: "/store/my-skill",
					LinkedTo:  []string{"/home/user/.claude/skills/my-skill"},
				},
			},
		}
		if diff := cmp.Diff(want, lf); diff != "" {
			t.Errorf("Load() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(path, []byte(":\tinvalid:"), 0o644); err != nil {
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
		path := filepath.Join(dir, "lock.yaml")
		want := &types.LockFile{
			Skills: []types.InstalledSkill{
				{Name: "s1", Repo: "github.com/a/b", Commit: "sha1", SkillPath: "/store/s1", LinkedTo: []string{"/link1"}},
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
}

func TestFindByName(t *testing.T) {
	lf := &types.LockFile{
		Skills: []types.InstalledSkill{
			{Name: "alpha", Repo: "github.com/a/a"},
			{Name: "beta", Repo: "github.com/b/b"},
		},
	}

	tests := []struct {
		name   string
		search string
		found  bool
	}{
		{"found first", "alpha", true},
		{"found second", "beta", true},
		{"not found", "gamma", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindByName(lf, tt.search)
			if tt.found && got == nil {
				t.Errorf("FindByName(%q) = nil, want non-nil", tt.search)
			}
			if !tt.found && got != nil {
				t.Errorf("FindByName(%q) = %v, want nil", tt.search, got)
			}
			if got != nil && got.Name != tt.search {
				t.Errorf("FindByName(%q).Name = %q, want %q", tt.search, got.Name, tt.search)
			}
		})
	}

	t.Run("empty lock file", func(t *testing.T) {
		empty := &types.LockFile{}
		if got := FindByName(empty, "any"); got != nil {
			t.Errorf("expected nil for empty LockFile, got %v", got)
		}
	})
}
