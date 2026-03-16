package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/types"
)

func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    skillFrontmatter
		wantErr bool
	}{
		{
			name:  "valid with name and description",
			input: "---\nname: my-skill\ndescription: A test skill\n---\n# Content",
			want:  skillFrontmatter{Name: "my-skill", Description: "A test skill"},
		},
		{
			name:  "valid with name only",
			input: "---\nname: only-name\n---\n",
			want:  skillFrontmatter{Name: "only-name"},
		},
		{
			name:    "no frontmatter - heading first",
			input:   "# Just a heading\nsome text",
			wantErr: true,
		},
		{
			name:  "empty file returns empty frontmatter no error",
			input: "",
			want:  skillFrontmatter{},
		},
		{
			name:  "no closing delimiter returns partial frontmatter",
			input: "---\nname: unclosed\ndescription: no end",
			want:  skillFrontmatter{Name: "unclosed", Description: "no end"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFrontmatter([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("extractFrontmatter() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("extractFrontmatter() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScan(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})

	t.Run("single skill with frontmatter name", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: frontend-skill\ndescription: A skill\n---\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := []types.DetectedSkill{
			{Name: "frontend-skill", Description: "A skill", SkillPath: skillDir},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Scan() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("single skill without frontmatter name uses dir name", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# No frontmatter"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(got))
		}
		if got[0].Name != "my-skill" {
			t.Errorf("expected name 'my-skill', got %q", got[0].Name)
		}
	})

	t.Run("multiple skills at same level", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"skill-a", "skill-b"} {
			skillDir := filepath.Join(dir, name)
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				t.Fatal(err)
			}
			content := "---\nname: " + name + "\n---\n"
			if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 skills, got %d: %v", len(got), got)
		}
	})

	t.Run("nested: does not descend into skill dir", func(t *testing.T) {
		dir := t.TempDir()
		outerDir := filepath.Join(dir, "outer")
		if err := os.MkdirAll(outerDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outerDir, "SKILL.md"), []byte("---\nname: outer\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// nested SKILL.md inside outer — must NOT be found
		innerDir := filepath.Join(outerDir, "inner")
		if err := os.MkdirAll(innerDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(innerDir, "SKILL.md"), []byte("---\nname: inner\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d: %v", len(got), got)
		}
		if got[0].Name != "outer" {
			t.Errorf("expected 'outer', got %q", got[0].Name)
		}
	})

	t.Run("nested: descends when no SKILL.md at intermediate level", func(t *testing.T) {
		dir := t.TempDir()
		// no SKILL.md in intermediate "group" dir
		deepDir := filepath.Join(dir, "group", "deep-skill")
		if err := os.MkdirAll(deepDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(deepDir, "SKILL.md"), []byte("---\nname: deep\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 skill, got %d: %v", len(got), got)
		}
		if got[0].Name != "deep" {
			t.Errorf("expected 'deep', got %q", got[0].Name)
		}
	})

	t.Run("deduplication: same name at different depths", func(t *testing.T) {
		dir := t.TempDir()

		// Shallow: planning-with-files/SKILL.md (depth 1)
		shallowDir := filepath.Join(dir, "planning-with-files")
		if err := os.MkdirAll(shallowDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(shallowDir, "SKILL.md"), []byte("---\nname: planning-with-files\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Deep: group/planning-with-files/SKILL.md (depth 2)
		deepDir := filepath.Join(dir, "group", "planning-with-files")
		if err := os.MkdirAll(deepDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(deepDir, "SKILL.md"), []byte("---\nname: planning-with-files\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := []types.DetectedSkill{
			{Name: "planning-with-files", SkillPath: shallowDir},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Scan() mismatch (-want +got):\n%s", diff)
		}
	})
}
