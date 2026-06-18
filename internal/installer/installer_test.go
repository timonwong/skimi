package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/lock"
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

func TestUpdateReposUpdatesSelectedRepoAndPreservesUnrelatedEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origin := filepath.Join(dir, "origin")
	makeSkillRepo(t, origin, map[string]string{
		"alpha": "alpha v1",
		"beta":  "beta v1",
	})
	oldCommit := gitHead(t, origin)

	repoID := "github.com/example/selected"
	storeDir := filepath.Join(dir, "store")
	storeRepo := RepoStorePath(storeDir, repoID)
	gitRun(t, dir, "clone", origin, storeRepo)

	makeSkillRepoCommit(t, origin, map[string]string{
		"alpha": "alpha v2",
		"beta":  "beta v2",
	})
	newCommit := gitHead(t, origin)

	staleLink := filepath.Join(dir, "stale-alpha")
	if err := os.WriteFile(staleLink, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(dir, "skills-lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{
		{
			Name:      "alpha",
			Repo:      repoID,
			Commit:    oldCommit,
			SkillPath: filepath.Join(storeRepo, "alpha"),
			LinkedTo:  []string{staleLink},
		},
		{
			Name:      "other",
			Repo:      "github.com/example/other",
			Commit:    "other-commit",
			SkillPath: "/store/other",
			LinkedTo:  []string{"/links/other"},
		},
		{
			Name:      "local",
			LocalPath: "/tmp/local",
			SkillPath: "/tmp/local/local",
			LinkedTo:  []string{"/links/local"},
		},
	}})

	cfg := &types.SkmConfig{
		Agents: &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{
			{Repo: repoID, Skills: []string{"alpha", "beta"}, TargetDir: "team"},
			{Repo: "github.com/example/other", Skills: []string{"other"}},
		},
	}

	if err := UpdateRepos(cfg, []string{repoID}, Options{
		StoreDir: storeDir,
		LockPath: lockPath,
	}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	got := readLock(t, lockPath)
	wantUnchanged := types.InstalledSkill{
		Name:      "other",
		Repo:      "github.com/example/other",
		Commit:    "other-commit",
		SkillPath: "/store/other",
		LinkedTo:  []string{"/links/other"},
	}
	if diff := cmp.Diff(wantUnchanged, got.Skills[0]); diff != "" {
		t.Errorf("unrelated remote entry mismatch (-want +got):\n%s", diff)
	}
	if got.Skills[1].Name != "local" || got.Skills[1].LocalPath != "/tmp/local" {
		t.Fatalf("local entry changed unexpectedly: %+v", got.Skills[1])
	}
	if len(got.Skills) != 4 {
		t.Fatalf("lock entries = %d, want 4: %+v", len(got.Skills), got.Skills)
	}

	var alpha, beta *types.InstalledSkill
	for i := range got.Skills {
		switch got.Skills[i].Name {
		case "alpha":
			alpha = &got.Skills[i]
		case "beta":
			beta = &got.Skills[i]
		}
	}
	if alpha == nil || beta == nil {
		t.Fatalf("updated lock missing alpha or beta: %+v", got.Skills)
	}
	if alpha.Commit != newCommit || beta.Commit != newCommit {
		t.Fatalf("updated commits = alpha:%q beta:%q, want %q", alpha.Commit, beta.Commit, newCommit)
	}
	if _, err := os.Lstat(staleLink); !os.IsNotExist(err) {
		t.Fatalf("stale link still exists: %v", err)
	}
}

func TestUpdateReposUpToDateLeavesLockStable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origin := filepath.Join(dir, "origin")
	makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})
	commit := gitHead(t, origin)

	repoID := "github.com/example/current"
	storeDir := filepath.Join(dir, "store")
	storeRepo := RepoStorePath(storeDir, repoID)
	gitRun(t, dir, "clone", origin, storeRepo)

	lockPath := filepath.Join(dir, "skills-lock.yaml")
	want := &types.LockFile{Skills: []types.InstalledSkill{
		{
			Name:      "alpha",
			Repo:      repoID,
			Commit:    commit,
			SkillPath: filepath.Join(storeRepo, "alpha"),
			LinkedTo:  []string{filepath.Join(dir, "alpha-link")},
		},
	}}
	writeLock(t, lockPath, want)

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: []string{"alpha"}}},
	}

	if err := UpdateRepos(cfg, []string{repoID}, Options{
		StoreDir: storeDir,
		LockPath: lockPath,
	}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	got := readLock(t, lockPath)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("lock mismatch (-want +got):\n%s", diff)
	}
}

func TestReplaceUpdatedRepoEntriesAppendsChangedReposInRequestOrder(t *testing.T) {
	lf := &types.LockFile{Skills: []types.InstalledSkill{
		{Name: "old-a", Repo: "repo-a"},
		{Name: "keep", Repo: "repo-keep"},
		{Name: "old-b", Repo: "repo-b"},
	}}
	changed := map[string][]types.InstalledSkill{
		"repo-a": {{Name: "new-a", Repo: "repo-a"}},
		"repo-b": {{Name: "new-b", Repo: "repo-b"}},
	}

	got := replaceUpdatedRepoEntries(lf, []string{"repo-b", "repo-a"}, changed)
	var names []string
	for _, skill := range got.Skills {
		names = append(names, skill.Name)
	}
	want := []string{"keep", "new-b", "new-a"}
	if diff := cmp.Diff(want, names); diff != "" {
		t.Errorf("skill order mismatch (-want +got):\n%s", diff)
	}
}

func makeSkillRepo(t *testing.T, dir string, skills map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test User")
	makeSkillRepoCommit(t, dir, skills)
}

func makeSkillRepoCommit(t *testing.T, dir string, skills map[string]string) {
	t.Helper()
	for name, content := range skills {
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := []byte("---\nname: " + name + "\ndescription: test skill\n---\n\n" + content + "\n")
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "update skills")
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	out := gitRun(t, dir, "rev-parse", "HEAD")
	return string(bytesTrimSpace(out))
}

func gitRun(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return out
}

func writeLock(t *testing.T, path string, lf *types.LockFile) {
	t.Helper()
	if err := lock.Save(path, lf); err != nil {
		t.Fatal(err)
	}
}

func readLock(t *testing.T, path string) *types.LockFile {
	t.Helper()
	lf, err := lock.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

func bytesTrimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\t' || b[0] == '\r') {
		b = b[1:]
	}
	for len(b) > 0 {
		last := b[len(b)-1]
		if last != ' ' && last != '\n' && last != '\t' && last != '\r' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
