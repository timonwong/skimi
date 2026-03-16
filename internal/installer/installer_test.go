package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/types"
)

func TestRepoStorePath(t *testing.T) {
	store := "/store"
	tests := []struct {
		repo string
		want string
	}{
		{"github.com/foo/bar", "/store/github.com/foo/bar"},
		{"https://github.com/foo/bar", "/store/github.com/foo/bar"},
		{"http://github.com/foo/bar", "/store/github.com/foo/bar"},
		{"git@github.com:foo/bar", "/store/github.com/foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			got := RepoStorePath(store, tt.repo)
			if got != tt.want {
				t.Errorf("RepoStorePath(%q, %q) = %q, want %q", store, tt.repo, got, tt.want)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tilde expansion",
			input: "~/foo",
			want:  filepath.Join(home, "foo"),
		},
		{
			name:  "absolute path unchanged",
			input: "/tmp/bar",
			want:  "/tmp/bar",
		},
		{
			name:  "relative path becomes absolute",
			input: "relative/path",
			want:  filepath.Join(cwd, "relative/path"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if err != nil {
				t.Fatalf("ExpandPath(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFilterSkills(t *testing.T) {
	all := []types.DetectedSkill{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}

	tests := []struct {
		name   string
		filter []string
		want   []types.DetectedSkill
	}{
		{
			name:   "subset match",
			filter: []string{"alpha", "gamma"},
			want:   []types.DetectedSkill{{Name: "alpha"}, {Name: "gamma"}},
		},
		{
			name:   "single match",
			filter: []string{"beta"},
			want:   []types.DetectedSkill{{Name: "beta"}},
		},
		{
			name:   "no match",
			filter: []string{"delta"},
			want:   nil,
		},
		{
			name:   "empty filter",
			filter: []string{},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterSkills(all, tt.filter)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("filterSkills() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveDefaultAgents(t *testing.T) {
	t.Run("nil agents returns AllAgents", func(t *testing.T) {
		cfg := &types.SkmConfig{Agents: nil}
		got := resolveDefaultAgents(cfg)
		if len(got) != len(types.AllAgents) {
			t.Errorf("expected AllAgents (%d), got %d: %v", len(types.AllAgents), len(got), got)
		}
	})

	t.Run("empty default returns AllAgents", func(t *testing.T) {
		cfg := &types.SkmConfig{Agents: &types.DefaultAgentsConfig{Default: []string{}}}
		got := resolveDefaultAgents(cfg)
		if len(got) != len(types.AllAgents) {
			t.Errorf("expected AllAgents (%d), got %d: %v", len(types.AllAgents), len(got), got)
		}
	})

	t.Run("configured default agents returned as-is", func(t *testing.T) {
		want := []string{types.AgentClaude, types.AgentCodex}
		cfg := &types.SkmConfig{Agents: &types.DefaultAgentsConfig{Default: want}}
		got := resolveDefaultAgents(cfg)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("resolveDefaultAgents() mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestResolvePackageAgents(t *testing.T) {
	defaults := []string{types.AgentClaude, types.AgentStandard, types.AgentCodex}

	tests := []struct {
		name string
		pkg  types.SkillPackageConfig
		want []string
	}{
		{
			name: "nil agents returns defaults unchanged",
			pkg:  types.SkillPackageConfig{Agents: nil},
			want: defaults,
		},
		{
			name: "includes overrides defaults",
			pkg: types.SkillPackageConfig{
				Agents: &types.AgentsConfig{Includes: []string{types.AgentClaude}},
			},
			want: []string{types.AgentClaude},
		},
		{
			name: "excludes removes from defaults",
			pkg: types.SkillPackageConfig{
				Agents: &types.AgentsConfig{Excludes: []string{types.AgentStandard}},
			},
			want: []string{types.AgentClaude, types.AgentCodex},
		},
		{
			name: "includes then excludes",
			pkg: types.SkillPackageConfig{
				Agents: &types.AgentsConfig{
					Includes: []string{types.AgentClaude, types.AgentCodex},
					Excludes: []string{types.AgentCodex},
				},
			},
			want: []string{types.AgentClaude},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePackageAgents(tt.pkg, defaults)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("resolvePackageAgents() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
