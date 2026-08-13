package cli

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/types"
)

func TestUpdateHintUsesAllFlag(t *testing.T) {
	got := updateHint()
	want := "Run `skimi update --all` to apply updates."
	if got != want {
		t.Fatalf("updateHint() = %q, want %q", got, want)
	}
}

func TestNormalizeCheckUpdateRepos(t *testing.T) {
	tests := []struct {
		name    string
		pkgs    []types.SkillPackageConfig
		want    []string
		wantErr string
	}{
		{
			name: "shorthand owner/repo expands to github.com",
			pkgs: []types.SkillPackageConfig{
				{Repo: "owner/repo"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "https URL normalizes to domain form",
			pkgs: []types.SkillPackageConfig{
				{Repo: "https://github.com/owner/repo"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "https URL with .git suffix drops the suffix",
			pkgs: []types.SkillPackageConfig{
				{Repo: "https://github.com/owner/repo.git"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "git@ SSH form normalizes to domain form",
			pkgs: []types.SkillPackageConfig{
				{Repo: "git@github.com:owner/repo.git"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "subdir form strips subdir from the repo identifier",
			pkgs: []types.SkillPackageConfig{
				{Repo: "github.com/owner/repo/path/to/skill"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "shorthand with subdir strips subdir",
			pkgs: []types.SkillPackageConfig{
				{Repo: "owner/repo/subdir"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "different spellings of the same repo dedupe to one entry",
			pkgs: []types.SkillPackageConfig{
				{Repo: "owner/repo/skill-a"},
				{Repo: "https://github.com/owner/repo.git"},
				{Repo: "git@github.com:owner/repo.git"},
				{Repo: "github.com/owner/repo/skill-b"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "distinct repos are preserved in first-seen order",
			pkgs: []types.SkillPackageConfig{
				{Repo: "owner/two"},
				{Repo: "owner/one"},
			},
			want: []string{"github.com/owner/two", "github.com/owner/one"},
		},
		{
			name: "packages without a repo are skipped",
			pkgs: []types.SkillPackageConfig{
				{LocalPath: "/tmp/local"},
				{Repo: "owner/repo"},
			},
			want: []string{"github.com/owner/repo"},
		},
		{
			name: "local path sources are skipped",
			pkgs: []types.SkillPackageConfig{
				{Repo: "./local/skill"},
				{Repo: "owner/repo"},
			},
			want: []string{"github.com/owner/repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCheckUpdateRepos(tt.pkgs)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("normalizeCheckUpdateRepos() error = nil, want error containing %q", tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeCheckUpdateRepos() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("normalizeCheckUpdateRepos() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
