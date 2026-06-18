package cli

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/types"
)

func TestSelectUpdateRepos(t *testing.T) {
	lf := &types.LockFile{
		Skills: []types.InstalledSkill{
			{Name: "alpha", Repo: "github.com/example/one"},
			{Name: "beta", Repo: "github.com/example/one"},
			{Name: "gamma", Repo: "github.com/example/two"},
			{Name: "local", LocalPath: "/tmp/local"},
		},
	}

	tests := []struct {
		name      string
		skills    []string
		updateAll bool
		wantRepos []string
		wantLocal []string
		wantErr   string
	}{
		{
			name:      "deduplicates repo for multiple skills",
			skills:    []string{"alpha", "beta"},
			wantRepos: []string{"github.com/example/one"},
		},
		{
			name:      "preserves repo order from skill arguments",
			skills:    []string{"gamma", "alpha"},
			wantRepos: []string{"github.com/example/two", "github.com/example/one"},
		},
		{
			name:      "all selects remote repos from lock",
			updateAll: true,
			wantRepos: []string{"github.com/example/one", "github.com/example/two"},
		},
		{
			name:      "local path skill is skipped",
			skills:    []string{"local"},
			wantLocal: []string{"local"},
		},
		{
			name:    "missing skill returns an error",
			skills:  []string{"missing"},
			wantErr: "Skill 'missing' is not installed.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectUpdateRepos(lf, tt.skills, tt.updateAll)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("selectUpdateRepos() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("selectUpdateRepos() error = %q, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectUpdateRepos() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.wantRepos, got.Repos); diff != "" {
				t.Errorf("Repos mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantLocal, got.LocalSkills); diff != "" {
				t.Errorf("LocalSkills mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUpdateCommandUsageValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "bare update requires selection",
			args: nil,
		},
		{
			name: "all and skill names are mutually exclusive",
			args: []string{"--all", "alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newUpdateCmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("newUpdateCmd().Execute() error = nil")
			}
			if !strings.Contains(err.Error(), "provide skill name(s) or use --all") {
				t.Fatalf("error = %q", err)
			}
		})
	}
}
