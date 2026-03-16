package linker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timonwong/skimi/internal/types"
)

func TestUseHardlink(t *testing.T) {
	tests := []struct {
		agent string
		want  bool
	}{
		{types.AgentStandard, true},
		{types.AgentOpenClaw, true},
		{types.AgentClaude, false},
		{types.AgentCodex, false},
		{types.AgentPi, false},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			got := useHardlink(tt.agent)
			if got != tt.want {
				t.Errorf("useHardlink(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}

func TestSkillLinkPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		agent     string
		targetDir string
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
			name:      "claude with targetDir",
			agent:     types.AgentClaude, targetDir: "sub", skillName: "my-skill",
			want: filepath.Join(home, ".claude", "skills", "sub", "my-skill"),
		},
		{
			name:    "unknown agent returns error",
			agent:   "unknown-agent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SkillLinkPath(tt.agent, tt.targetDir, tt.skillName)
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

	if err := CreateLink(srcDir, dstPath, types.AgentClaude); err != nil {
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

func TestCreateLink_Hardlink(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"file1.txt": "hello",
		"file2.txt": "world",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dstDir := filepath.Join(dir, "dst")
	if err := CreateLink(srcDir, dstDir, types.AgentStandard); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	for name, wantContent := range files {
		dstFile := filepath.Join(dstDir, name)
		gotContent, err := os.ReadFile(dstFile)
		if err != nil {
			t.Fatalf("read %s: %v", dstFile, err)
		}
		if string(gotContent) != wantContent {
			t.Errorf("file %s: content = %q, want %q", name, gotContent, wantContent)
		}

		srcFi, err := os.Stat(filepath.Join(srcDir, name))
		if err != nil {
			t.Fatal(err)
		}
		dstFi, err := os.Stat(dstFile)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(srcFi, dstFi) {
			t.Errorf("file %s: not a hardlink (inode differs)", name)
		}
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := "test content"

	if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}

	srcFi, err := os.Stat(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	dstFi, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if srcFi.Mode() != dstFi.Mode() {
		t.Errorf("mode mismatch: src=%v dst=%v", srcFi.Mode(), dstFi.Mode())
	}
}
