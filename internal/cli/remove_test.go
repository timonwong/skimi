package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timonwong/skimi/internal/types"
)

func TestRemoveSkillsVerifiesOwnership(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "src", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(dir, "links")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}

	managed := filepath.Join(linkDir, "alpha")
	if err := os.Symlink(skillDir, managed); err != nil {
		t.Fatal(err)
	}
	usurped := filepath.Join(linkDir, "beta")
	if err := os.MkdirAll(usurped, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(usurped, "NOTES.md")
	if err := os.WriteFile(userFile, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}

	lf := &types.LockFile{Skills: []types.InstalledSkill{
		{Name: "alpha", SkillPath: skillDir, LinkedTo: []string{managed}},
		{Name: "beta", SkillPath: filepath.Join(dir, "src", "beta"), LinkedTo: []string{usurped}},
		{Name: "keep", SkillPath: filepath.Join(dir, "src", "keep"), LinkedTo: []string{filepath.Join(linkDir, "keep")}},
	}}

	remaining, removed := removeSkills(lf, []string{"alpha", "beta"})
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	if len(remaining) != 1 || remaining[0].Name != "keep" {
		t.Fatalf("remaining = %+v", remaining)
	}
	if _, err := os.Lstat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed link still exists: %v", err)
	}
	data, err := os.ReadFile(userFile)
	if err != nil || string(data) != "user data" {
		t.Fatalf("user directory changed: %q %v", data, err)
	}
}
