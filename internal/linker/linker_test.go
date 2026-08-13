package linker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timonwong/skimi/internal/types"
)

func TestSkillLinkPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		agent     string
		skillName string
		wantErr   bool
		want      string
	}{
		{
			name:  "claude no targetDir",
			agent: types.AgentClaude, skillName: "my-skill",
			want: filepath.Join(home, ".claude", "skills", "my-skill"),
		},
		{
			name:  "standard no targetDir",
			agent: types.AgentStandard, skillName: "my-skill",
			want: filepath.Join(home, ".agents", "skills", "my-skill"),
		},
		{
			name:  "codex no targetDir",
			agent: types.AgentCodex, skillName: "my-skill",
			want: filepath.Join(home, ".codex", "skills", "my-skill"),
		},
		{
			name:  "openclaw no targetDir",
			agent: types.AgentOpenClaw, skillName: "my-skill",
			want: filepath.Join(home, ".openclaw", "skills", "my-skill"),
		},
		{
			name:  "pi no targetDir",
			agent: types.AgentPi, skillName: "my-skill",
			want: filepath.Join(home, ".pi", "agent", "skills", "my-skill"),
		},
		{
			name:    "unknown agent returns error",
			agent:   "unknown-agent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SkillLinkPath(tt.agent, tt.skillName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SkillLinkPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("SkillLinkPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateAndRemoveLink_Symlink(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dstPath := filepath.Join(dir, "link")

	if err := CreateLink(srcDir, dstPath); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	fi, err := os.Lstat(dstPath)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink, got mode %v", fi.Mode())
	}

	if err := RemoveLink(dstPath); err != nil {
		t.Fatalf("RemoveLink: %v", err)
	}
	if _, err := os.Lstat(dstPath); !os.IsNotExist(err) {
		t.Errorf("expected path to not exist after RemoveLink")
	}
}

func TestCreateLink_StandardUsesSymlink(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(dir, "dst")
	if err := CreateLink(srcDir, dstDir); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	fi, err := os.Lstat(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink, got %v", fi.Mode())
	}
}
