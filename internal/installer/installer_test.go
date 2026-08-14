package installer

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/lock"
	"github.com/timonwong/skimi/internal/types"
)

func selectors(names ...string) []types.SkillSelector {
	out := make([]types.SkillSelector, len(names))
	for i, name := range names {
		out[i] = types.SkillSelector{Name: name}
	}
	return out
}

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

func TestSelectSkills(t *testing.T) {
	all := []types.DetectedSkill{
		{Name: "alpha", SourcePath: "group/alpha"},
		{Name: "beta", SourcePath: "beta"},
		{Name: "alpha", SourcePath: "other/alpha"},
	}
	want := []types.DetectedSkill{
		{Name: "alpha", SourcePath: "group/alpha"},
		{Name: "beta", SourcePath: "beta"},
		{Name: "alpha", SourcePath: "other/alpha"},
	}
	got, err := selectSkills(all, []types.SkillSelector{{Name: "alpha"}, {Path: "beta"}})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("selectSkills() mismatch (-want +got):\n%s", diff)
	}
	if _, err := selectSkills(all, []types.SkillSelector{{Path: "missing"}}); err == nil {
		t.Fatal("selectSkills() expected missing selector error")
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
	if err := os.Symlink(filepath.Join(storeRepo, "alpha"), staleLink); err != nil {
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
			{Repo: repoID, Skills: selectors("alpha", "beta"), TargetDir: "team"},
			{Repo: "github.com/example/other", Skills: selectors("other")},
		},
	}

	if err := UpdateRepos(cfg, []string{repoID}, Options{
		StoreDir: storeDir,
		LockPath: lockPath,
	}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	got := readLock(t, lockPath)
	byName := make(map[string]types.InstalledSkill, len(got.Skills))
	for _, skill := range got.Skills {
		byName[skill.Name] = skill
	}
	wantUnchanged := types.InstalledSkill{
		Name:      "other",
		Repo:      "github.com/example/other",
		Commit:    "other-commit",
		SkillPath: "/store/other",
		LinkedTo:  []string{"/links/other"},
	}
	if diff := cmp.Diff(wantUnchanged, byName["other"]); diff != "" {
		t.Errorf("unrelated remote entry mismatch (-want +got):\n%s", diff)
	}
	if byName["local"].LocalPath != "/tmp/local" {
		t.Fatalf("local entry changed unexpectedly: %+v", byName["local"])
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

func TestUpdateReposUpToDateMigratesV1Lock(t *testing.T) {
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
		Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
	}

	if err := UpdateRepos(cfg, []string{repoID}, Options{
		StoreDir: storeDir,
		LockPath: lockPath,
	}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	got := readLock(t, lockPath)
	if got.Version != lock.CurrentVersion || len(got.Skills) != 1 {
		t.Fatalf("lock = %+v", got)
	}
	if got.Skills[0].Commit != commit || got.Skills[0].SourcePath != "alpha" {
		t.Fatalf("migrated skill = %+v", got.Skills[0])
	}
	newLink := filepath.Join(dir, "home", ".claude", "skills", "alpha")
	if fi, err := os.Lstat(newLink); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("migrated link: %v %v", fi, err)
	}
}

// TestRunSkipSyncControlsRemoteSync pins the single-sync contract the
// interactive install relies on: with SkipSync the store copy is installed as
// it is, and the zero value still clones or pulls as before.
func TestRunSkipSyncControlsRemoteSync(t *testing.T) {
	tests := []struct {
		name          string
		skipSync      bool
		breakRemote   bool
		wantErr       bool
		wantNewCommit bool
	}{
		{name: "skip sync installs from the cached copy while offline", skipSync: true, breakRemote: true},
		{name: "full sync fails when the remote is unreachable", skipSync: false, breakRemote: true, wantErr: true},
		{name: "skip sync keeps the cached commit", skipSync: true},
		{name: "full sync pulls the new commit", skipSync: false, wantNewCommit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			home := filepath.Join(dir, "home")
			t.Setenv("HOME", home)

			origin := filepath.Join(dir, "origin")
			makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})

			repoID := "github.com/example/cached"
			storeDir := filepath.Join(dir, "store")
			storeRepo := RepoStorePath(storeDir, repoID)
			gitRun(t, dir, "clone", origin, storeRepo)
			cachedCommit := gitHead(t, storeRepo)

			makeSkillRepoCommit(t, origin, map[string]string{"alpha": "alpha v2"})
			newCommit := gitHead(t, origin)
			if tt.breakRemote {
				// Any pull now fails loudly, so a successful Run proves none ran.
				gitRun(t, storeRepo, "remote", "set-url", "origin", filepath.Join(dir, "missing-origin"))
			}

			lockPath := filepath.Join(dir, "lock.yaml")
			cfg := &types.SkmConfig{
				Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
				Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
			}

			err := Run(cfg, Options{
				StoreDir: storeDir,
				LockPath: lockPath,
				Additive: true,
				SkipSync: tt.skipSync,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("Run() expected an error when the pull fails")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			lf := readLock(t, lockPath)
			if len(lf.Skills) != 1 || lf.Skills[0].Name != "alpha" {
				t.Fatalf("lock entries = %+v, want a single alpha entry", lf.Skills)
			}
			wantCommit, wantBody := cachedCommit, "alpha v1"
			if tt.wantNewCommit {
				wantCommit, wantBody = newCommit, "alpha v2"
			}
			if lf.Skills[0].Commit != wantCommit {
				t.Fatalf("locked commit = %q, want %q", lf.Skills[0].Commit, wantCommit)
			}
			link := filepath.Join(home, ".claude", "skills", "alpha")
			fi, err := os.Lstat(link)
			if err != nil || fi.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("alpha link: %v %v", fi, err)
			}
			body, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), wantBody) {
				t.Fatalf("linked SKILL.md = %q, want it to contain %q", body, wantBody)
			}
		})
	}
}

// TestRunDryRunFetchesButNeverPullsCachedStore pins the issue #17 contract: a
// dry run against a repo that is already cloned may fetch it to preview
// updates, but must never pull, since installed skills are symlinks into the
// store's working tree and a pull would move what agents load before the
// lock records anything.
func TestRunDryRunFetchesButNeverPullsCachedStore(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	origin := filepath.Join(dir, "origin")
	makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})

	repoID := "github.com/example/dryrun-cached"
	storeDir := filepath.Join(dir, "store")
	storeRepo := RepoStorePath(storeDir, repoID)
	gitRun(t, dir, "clone", origin, storeRepo)
	cachedCommit := gitHead(t, storeRepo)

	// Advance the origin so a real sync would move the store past cachedCommit.
	makeSkillRepoCommit(t, origin, map[string]string{"alpha": "alpha v2"})

	lockPath := filepath.Join(dir, "lock.yaml")
	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
	}

	if err := Run(cfg, Options{
		StoreDir: storeDir,
		LockPath: lockPath,
		DryRun:   true,
	}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if got := gitHead(t, storeRepo); got != cachedCommit {
		t.Fatalf("store HEAD moved during dry run: got %q, want unchanged %q", got, cachedCommit)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote a lock file: stat err = %v", err)
	}
	link := filepath.Join(home, ".claude", "skills", "alpha")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("dry run created a link at %s: stat err = %v", link, err)
	}
}

// TestRunDryRunReportsWouldCloneForMissingStore pins the other half of the
// issue #17 fix: a dry run for a repo that has never been cloned has nothing
// local to detect skills from, so it must report the clone and skip that
// package instead of failing the whole dry run.
func TestRunDryRunReportsWouldCloneForMissingStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	repoID := "github.com/example/dryrun-never-cloned"
	storeDir := filepath.Join(dir, "store")
	lockPath := filepath.Join(dir, "lock.yaml")

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
	}

	var runErr error
	stdout := captureStdout(t, func() {
		runErr = Run(cfg, Options{
			StoreDir: storeDir,
			LockPath: lockPath,
			DryRun:   true,
		})
	})
	if runErr != nil {
		t.Fatalf("Run() error: %v", runErr)
	}

	if !strings.Contains(stdout, "Would clone "+repoID) {
		t.Fatalf("stdout = %q, want it to contain %q", stdout, "Would clone "+repoID)
	}
	if _, err := os.Stat(RepoStorePath(storeDir, repoID)); !os.IsNotExist(err) {
		t.Fatalf("dry run cloned the repo: stat err = %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote a lock file: stat err = %v", err)
	}
}

// TestUpdateReposDryRunFetchesButNeverPulls mirrors the Run() dry-run
// contract for UpdateRepos: its up-front sync loop must fetch to preview
// updates instead of pulling every selected repo before applyPlan ever
// consults opts.DryRun.
func TestUpdateReposDryRunFetchesButNeverPulls(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	origin := filepath.Join(dir, "origin")
	makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})
	oldCommit := gitHead(t, origin)

	repoID := "github.com/example/dryrun-update"
	storeDir := filepath.Join(dir, "store")
	storeRepo := RepoStorePath(storeDir, repoID)
	gitRun(t, dir, "clone", origin, storeRepo)

	makeSkillRepoCommit(t, origin, map[string]string{"alpha": "alpha v2"})

	lockPath := filepath.Join(dir, "skills-lock.yaml")
	alphaLink := filepath.Join(home, ".claude", "skills", "alpha")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name:      "alpha",
		Repo:      repoID,
		Commit:    oldCommit,
		SkillPath: filepath.Join(storeRepo, "alpha"),
		LinkedTo:  []string{alphaLink},
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
	}

	if err := UpdateRepos(cfg, []string{repoID}, Options{
		StoreDir: storeDir,
		LockPath: lockPath,
		DryRun:   true,
	}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	if got := gitHead(t, storeRepo); got != oldCommit {
		t.Fatalf("store HEAD moved during dry run: got %q, want unchanged %q", got, oldCommit)
	}
	got := readLock(t, lockPath)
	if len(got.Skills) != 1 || got.Skills[0].Commit != oldCommit {
		t.Fatalf("lock changed during dry run: %+v", got.Skills)
	}
	if _, err := os.Lstat(alphaLink); !os.IsNotExist(err) {
		t.Fatalf("dry run created a link at %s: stat err = %v", alphaLink, err)
	}
}

func TestRunFlattensAndUsesDeterministicLastWinner(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	sourceDir := filepath.Join(dir, "source")
	writeSkill(t, filepath.Join(sourceDir, "skills", "a", "wait"), "wait")
	writeSkill(t, filepath.Join(sourceDir, "skills", "z", "wait"), "wait")
	writeSkill(t, filepath.Join(sourceDir, "skills", "then"), "then")

	lockPath := filepath.Join(dir, "lock.yaml")
	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude, types.AgentStandard}},
		Packages: []types.SkillPackageConfig{{LocalPath: sourceDir}},
	}
	if err := Run(cfg, Options{LockPath: lockPath, StoreDir: filepath.Join(dir, "store")}); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".agents", "skills")} {
		wait := filepath.Join(root, "wait")
		fi, err := os.Lstat(wait)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", wait)
		}
		target, err := os.Readlink(wait)
		if err != nil {
			t.Fatal(err)
		}
		if target != filepath.Join(sourceDir, "skills", "z", "wait") {
			t.Fatalf("wait target = %q", target)
		}
	}
	lf := readLock(t, lockPath)
	if lf.Version != lock.CurrentVersion || len(lf.Skills) != 2 {
		t.Fatalf("lock = %+v", lf)
	}
}

func TestRunPathSelectorOverridesNameDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	sourceDir := filepath.Join(dir, "source")
	writeSkill(t, filepath.Join(sourceDir, "skills", "a", "wait"), "wait")
	writeSkill(t, filepath.Join(sourceDir, "skills", "z", "wait"), "wait")
	cfg := &types.SkmConfig{
		Agents: &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{
			LocalPath: sourceDir,
			Skills:    []types.SkillSelector{{Path: "a/wait"}},
		}},
	}
	if err := Run(cfg, Options{LockPath: filepath.Join(dir, "lock.yaml")}); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, "home", ".claude", "skills", "wait"))
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join(sourceDir, "skills", "a", "wait") {
		t.Fatalf("target = %q", target)
	}
}

func TestRunLaterPackageWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	writeSkill(t, filepath.Join(first, "wait"), "wait")
	writeSkill(t, filepath.Join(second, "wait"), "wait")
	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{LocalPath: first}, {LocalPath: second}},
	}
	if err := Run(cfg, Options{LockPath: filepath.Join(dir, "lock.yaml")}); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, "home", ".claude", "skills", "wait"))
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join(second, "wait") {
		t.Fatalf("target = %q", target)
	}
}

func TestRunMigratesV1NestedLinkAndRejectsUnownedDestination(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	sourceDir := filepath.Join(dir, "source")
	skillDir := filepath.Join(sourceDir, "wait")
	writeSkill(t, skillDir, "wait")
	oldLink := filepath.Join(home, ".claude", "skills", "legacy", "wait")
	if err := os.MkdirAll(filepath.Dir(oldLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillDir, oldLink); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name: "wait", LocalPath: sourceDir, SkillPath: skillDir, TargetDir: "legacy", LinkedTo: []string{oldLink},
	}}})
	cfg := &types.SkmConfig{Agents: &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}}, Packages: []types.SkillPackageConfig{{LocalPath: sourceDir}}}
	if err := Run(cfg, Options{LockPath: lockPath}); err != nil {
		t.Fatal(err)
	}
	newLink := filepath.Join(home, ".claude", "skills", "wait")
	if fi, err := os.Lstat(newLink); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("new link: %v %v", fi, err)
	}
	if _, err := os.Lstat(oldLink); !os.IsNotExist(err) {
		t.Fatalf("old link remains: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(oldLink)); !os.IsNotExist(err) {
		t.Fatalf("empty legacy dir remains: %v", err)
	}

	unowned := filepath.Join(home, ".claude", "skills", "wait")
	if err := os.Remove(unowned); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unowned, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeLock(t, lockPath, &types.LockFile{})
	if err := Run(cfg, Options{LockPath: lockPath}); err == nil {
		t.Fatal("expected unowned destination conflict")
	}
	data, err := os.ReadFile(unowned)
	if err != nil || string(data) != "user data" {
		t.Fatalf("unowned destination changed: %q %v", data, err)
	}
}

func TestRunAdditiveKeepsUnrelatedSkills(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	alphaSource := filepath.Join(dir, "alpha-source")
	alphaSkill := filepath.Join(alphaSource, "alpha")
	writeSkill(t, alphaSkill, "alpha")
	betaSource := filepath.Join(dir, "beta-source")
	writeSkill(t, filepath.Join(betaSource, "beta"), "beta")

	alphaLink := filepath.Join(home, ".claude", "skills", "alpha")
	if err := os.MkdirAll(filepath.Dir(alphaLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alphaSkill, alphaLink); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name: "alpha", LocalPath: alphaSource, SkillPath: alphaSkill, LinkedTo: []string{alphaLink},
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{LocalPath: betaSource}},
	}
	if err := Run(cfg, Options{LockPath: lockPath, Additive: true}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	target, err := os.Readlink(alphaLink)
	if err != nil || target != alphaSkill {
		t.Fatalf("unrelated alpha link changed: %q %v", target, err)
	}
	lf := readLock(t, lockPath)
	byName := make(map[string]types.InstalledSkill, len(lf.Skills))
	for _, skill := range lf.Skills {
		byName[skill.Name] = skill
	}
	if len(lf.Skills) != 2 {
		t.Fatalf("lock entries = %d, want 2: %+v", len(lf.Skills), lf.Skills)
	}
	if byName["alpha"].SkillPath != alphaSkill {
		t.Fatalf("alpha entry changed: %+v", byName["alpha"])
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "beta")); err != nil {
		t.Fatalf("beta link missing: %v", err)
	}
}

func TestRunAdditiveReplacesSameNameSkill(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	oldSource := filepath.Join(dir, "old-source")
	oldSkill := filepath.Join(oldSource, "wait")
	writeSkill(t, oldSkill, "wait")
	newSource := filepath.Join(dir, "new-source")
	newSkill := filepath.Join(newSource, "wait")
	writeSkill(t, newSkill, "wait")

	link := filepath.Join(home, ".claude", "skills", "wait")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oldSkill, link); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name: "wait", LocalPath: oldSource, SkillPath: oldSkill, LinkedTo: []string{link},
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{LocalPath: newSource}},
	}
	if err := Run(cfg, Options{LockPath: lockPath, Additive: true}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	target, err := os.Readlink(link)
	if err != nil || target != newSkill {
		t.Fatalf("link target = %q %v, want %q", target, err, newSkill)
	}
	lf := readLock(t, lockPath)
	if len(lf.Skills) != 1 || lf.Skills[0].SkillPath != newSkill {
		t.Fatalf("lock = %+v", lf.Skills)
	}
}

func TestRunRefusesToReplaceUserDirAtLockClaimedPath(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	sourceDir := filepath.Join(dir, "source")
	skillDir := filepath.Join(sourceDir, "wait")
	writeSkill(t, skillDir, "wait")

	dst := filepath.Join(home, ".claude", "skills", "wait")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(dst, "NOTES.md")
	if err := os.WriteFile(userFile, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name: "wait", LocalPath: sourceDir, SkillPath: skillDir, LinkedTo: []string{dst},
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{LocalPath: sourceDir}},
	}
	err := Run(cfg, Options{LockPath: lockPath})
	if err == nil || !strings.Contains(err.Error(), "not managed by skimi") {
		t.Fatalf("Run() error = %v, want unmanaged destination error", err)
	}
	data, readErr := os.ReadFile(userFile)
	if readErr != nil || string(data) != "user data" {
		t.Fatalf("user directory changed: %q %v", data, readErr)
	}
}

func TestRunLeavesUserDirWhenPruningStale(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	alphaSource := filepath.Join(dir, "alpha-source")
	writeSkill(t, filepath.Join(alphaSource, "alpha"), "alpha")
	betaSource := filepath.Join(dir, "beta-source")
	writeSkill(t, filepath.Join(betaSource, "beta"), "beta")

	alphaDst := filepath.Join(home, ".claude", "skills", "alpha")
	if err := os.MkdirAll(alphaDst, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(alphaDst, "NOTES.md")
	if err := os.WriteFile(userFile, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name: "alpha", LocalPath: alphaSource, SkillPath: filepath.Join(alphaSource, "alpha"), LinkedTo: []string{alphaDst},
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{LocalPath: betaSource}},
	}
	if err := Run(cfg, Options{LockPath: lockPath}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	data, err := os.ReadFile(userFile)
	if err != nil || string(data) != "user data" {
		t.Fatalf("user directory changed: %q %v", data, err)
	}
	lf := readLock(t, lockPath)
	if len(lf.Skills) != 1 || lf.Skills[0].Name != "beta" {
		t.Fatalf("lock = %+v", lf.Skills)
	}
}

func TestRunMigratesLegacyHardlinkTree(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	sourceDir := filepath.Join(dir, "source")
	skillDir := filepath.Join(sourceDir, "wait")
	writeSkill(t, skillDir, "wait")

	dst := filepath.Join(home, ".agents", "skills", "wait")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(skillDir, "SKILL.md"), filepath.Join(dst, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name: "wait", LocalPath: sourceDir, SkillPath: skillDir, LinkedTo: []string{dst},
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentStandard}},
		Packages: []types.SkillPackageConfig{{LocalPath: sourceDir}},
	}
	if err := Run(cfg, Options{LockPath: lockPath}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("legacy hardlink tree not migrated to symlink: %v", fi.Mode())
	}
	target, err := os.Readlink(dst)
	if err != nil || target != skillDir {
		t.Fatalf("migrated target = %q %v", target, err)
	}
}

func TestRunRollsBackLinksWhenLockWriteFails(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	sourceDir := filepath.Join(dir, "source")
	skillDir := filepath.Join(sourceDir, "wait")
	writeSkill(t, skillDir, "wait")
	destination := filepath.Join(home, ".claude", "skills", "wait")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	oldSource := filepath.Join(dir, "old-wait")
	writeSkill(t, oldSource, "wait")
	if err := os.Symlink(oldSource, destination); err != nil {
		t.Fatal(err)
	}
	lockDir := filepath.Join(dir, "readonly-lock-dir")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(lockDir, "lock.yaml")
	old := &types.LockFile{Skills: []types.InstalledSkill{{Name: "wait", LocalPath: sourceDir, SkillPath: oldSource, LinkedTo: []string{destination}}}}
	writeLock(t, lockPath, old)
	if err := os.Chmod(lockDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(lockDir, 0o755); err != nil {
			t.Errorf("restore lock directory permissions: %v", err)
		}
	})
	cfg := &types.SkmConfig{Agents: &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}}, Packages: []types.SkillPackageConfig{{LocalPath: sourceDir}}}
	if err := Run(cfg, Options{LockPath: lockPath}); err == nil {
		t.Fatal("expected lock write failure")
	}
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != oldSource {
		t.Fatalf("rollback target = %q, want %q", target, oldSource)
	}
}

func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\nname: " + name + "\ndescription: test\n---\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
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

// captureStdout runs fn with the process's os.Stdout redirected into a
// buffer and returns everything fn printed. installer prints status lines
// straight to os.Stdout, so tests asserting on dry-run reporting need to
// intercept it there rather than through an injectable writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	defer func() {
		os.Stdout = orig
		_ = w.Close()
	}()

	fn()

	_ = w.Close()
	return <-done
}

// cloneStore clones origin into storeRepo the way a healthy store copy looks.
func cloneStore(t *testing.T, origin, storeRepo string) {
	t.Helper()
	gitRun(t, filepath.Dir(origin), "clone", origin, storeRepo)
}

// redirectClones points cloneURL at a local origin through git's insteadOf
// rewriting, so tests exercise the real clone path without network access.
func redirectClones(t *testing.T, cloneURL, origin string) {
	t.Helper()
	gitconfig := filepath.Join(t.TempDir(), "gitconfig")
	body := "[url \"" + filepath.ToSlash(origin) + "\"]\n\tinsteadOf = " + cloneURL + "\n"
	if err := os.WriteFile(gitconfig, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
}

// rewriteHistory replaces the tip commit of dir, the local stand-in for an
// upstream force-push: the commit the store copy holds stops existing.
func rewriteHistory(t *testing.T, dir, skill, content string) {
	t.Helper()
	body := []byte("---\nname: " + skill + "\ndescription: test skill\n---\n\n" + content + "\n")
	if err := os.WriteFile(filepath.Join(dir, skill, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "commit", "-a", "--amend", "-m", "rewritten history")
}

func skillBody(t *testing.T, repoDir, skill string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoDir, skill, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestEnsureRepoSelfHeals pins the recovery contract for the store cache: a
// directory git cannot use is re-cloned, an upstream that was force-pushed is
// reset onto, and a remote that cannot be reached leaves the cached copy in
// place instead of deleting the only usable copy.
func TestEnsureRepoSelfHeals(t *testing.T) {
	tests := []struct {
		name string
		// prepare leaves the store copy in the state under test and may move
		// origin, which starts as a single commit holding "alpha v1".
		prepare func(t *testing.T, origin, storeRepo string)
		wantErr bool
		// wantBody is the SKILL.md content the store copy must hold afterwards.
		wantBody string
	}{
		{
			name: "empty directory is re-cloned",
			prepare: func(t *testing.T, _, storeRepo string) {
				if err := os.MkdirAll(storeRepo, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantBody: "alpha v1",
		},
		{
			name: "directory that is not a clone is re-cloned",
			prepare: func(t *testing.T, _, storeRepo string) {
				writeSkill(t, filepath.Join(storeRepo, "junk"), "junk")
			},
			wantBody: "alpha v1",
		},
		{
			name: "interrupted clone is re-cloned",
			prepare: func(t *testing.T, _, storeRepo string) {
				if err := os.MkdirAll(storeRepo, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(storeRepo, ".git"), []byte("truncated\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBody: "alpha v1",
		},
		{
			name: "healthy clone fast-forwards",
			prepare: func(t *testing.T, origin, storeRepo string) {
				cloneStore(t, origin, storeRepo)
				makeSkillRepoCommit(t, origin, map[string]string{"alpha": "alpha v2"})
			},
			wantBody: "alpha v2",
		},
		{
			name: "force-pushed upstream is reset onto",
			prepare: func(t *testing.T, origin, storeRepo string) {
				cloneStore(t, origin, storeRepo)
				rewriteHistory(t, origin, "alpha", "alpha rewritten")
			},
			wantBody: "alpha rewritten",
		},
		{
			name: "unreachable remote keeps the cached copy",
			prepare: func(t *testing.T, origin, storeRepo string) {
				cloneStore(t, origin, storeRepo)
				gitRun(t, storeRepo, "remote", "set-url", "origin", filepath.Join(filepath.Dir(origin), "missing-origin"))
				makeSkillRepoCommit(t, origin, map[string]string{"alpha": "alpha v2"})
			},
			wantErr:  true,
			wantBody: "alpha v1",
		},
	}

	const repoID = "github.com/example/healed"
	cloneURL := "https://" + repoID

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			origin := filepath.Join(dir, "origin")
			makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})
			redirectClones(t, cloneURL, origin)

			storeDir := filepath.Join(dir, "store")
			storeRepo := RepoStorePath(storeDir, repoID)
			tt.prepare(t, origin, storeRepo)

			err := EnsureRepo(storeDir, cloneURL, storeRepo)
			if tt.wantErr {
				if err == nil {
					t.Fatal("EnsureRepo() expected an error when the remote is unreachable")
				}
				if !strings.Contains(err.Error(), storeRepo) {
					t.Errorf("EnsureRepo() error = %v, want it to name the store path %s", err, storeRepo)
				}
			} else if err != nil {
				t.Fatalf("EnsureRepo() error: %v", err)
			}

			if !git.IsRepoRoot(storeRepo) {
				t.Fatalf("%s is not a usable clone after EnsureRepo()", storeRepo)
			}
			if body := skillBody(t, storeRepo, "alpha"); !strings.Contains(body, tt.wantBody) {
				t.Errorf("store SKILL.md = %q, want it to contain %q", body, tt.wantBody)
			}
			if !tt.wantErr && gitHead(t, storeRepo) != gitHead(t, origin) {
				t.Errorf("store HEAD = %q, want origin HEAD %q", gitHead(t, storeRepo), gitHead(t, origin))
			}
		})
	}
}

// TestEnsureRepoRefusesRecoveryOutsideStore keeps the destructive half of the
// recovery inside the directory skimi owns.
func TestEnsureRepoRefusesRecoveryOutsideStore(t *testing.T) {
	const repoID = "github.com/example/outside"
	cloneURL := "https://" + repoID

	dir := t.TempDir()
	origin := filepath.Join(dir, "origin")
	makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})
	redirectClones(t, cloneURL, origin)

	storeDir := filepath.Join(dir, "store")
	outside := filepath.Join(dir, "elsewhere", "github.com", "example", "outside")
	precious := filepath.Join(outside, "keepme")
	writeSkill(t, precious, "keepme")

	err := EnsureRepo(storeDir, cloneURL, outside)
	if err == nil {
		t.Fatal("EnsureRepo() expected a refusal for a destination outside the store")
	}
	if !strings.Contains(err.Error(), storeDir) || !strings.Contains(err.Error(), outside) {
		t.Errorf("EnsureRepo() error = %v, want it to name both %s and %s", err, outside, storeDir)
	}
	if _, statErr := os.Stat(filepath.Join(precious, "SKILL.md")); statErr != nil {
		t.Fatalf("recovery deleted files outside the store: %v", statErr)
	}
}

func TestInsideStore(t *testing.T) {
	store := filepath.Join(t.TempDir(), "store")
	tests := []struct {
		name  string
		store string
		dest  string
		want  bool
	}{
		{name: "repo below the store", store: store, dest: filepath.Join(store, "github.com", "foo", "bar"), want: true},
		{name: "store itself", store: store, dest: store},
		{name: "sibling of the store", store: store, dest: filepath.Join(filepath.Dir(store), "other")},
		{name: "parent of the store", store: store, dest: filepath.Dir(store)},
		{name: "escaping traversal", store: store, dest: filepath.Join(store, "..", "other")},
		{name: "empty store", store: "", dest: filepath.Join(store, "foo")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := insideStore(tt.store, tt.dest); got != tt.want {
				t.Errorf("insideStore(%q, %q) = %v, want %v", tt.store, tt.dest, got, tt.want)
			}
		})
	}
}

// TestRunHealsBrokenStoreClone drives the repair through the config-driven
// install, the path a broken store dir used to fail on every run.
func TestRunHealsBrokenStoreClone(t *testing.T) {
	tests := []struct {
		name       string
		breakStore func(t *testing.T, storeRepo string)
	}{
		{
			name: "empty store directory",
			breakStore: func(t *testing.T, storeRepo string) {
				if err := os.MkdirAll(storeRepo, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "store directory holding junk",
			breakStore: func(t *testing.T, storeRepo string) {
				writeSkill(t, filepath.Join(storeRepo, "junk"), "junk")
			},
		},
	}

	const repoID = "github.com/example/broken"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			home := filepath.Join(dir, "home")
			t.Setenv("HOME", home)

			origin := filepath.Join(dir, "origin")
			makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})
			redirectClones(t, "https://"+repoID, origin)

			storeDir := filepath.Join(dir, "store")
			storeRepo := RepoStorePath(storeDir, repoID)
			tt.breakStore(t, storeRepo)

			lockPath := filepath.Join(dir, "lock.yaml")
			cfg := &types.SkmConfig{
				Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
				Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
			}
			if err := Run(cfg, Options{StoreDir: storeDir, LockPath: lockPath}); err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			lf := readLock(t, lockPath)
			if len(lf.Skills) != 1 || lf.Skills[0].Name != "alpha" {
				t.Fatalf("lock entries = %+v, want a single alpha entry", lf.Skills)
			}
			if want := gitHead(t, origin); lf.Skills[0].Commit != want {
				t.Errorf("locked commit = %q, want %q", lf.Skills[0].Commit, want)
			}
			link := filepath.Join(home, ".claude", "skills", "alpha")
			if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("alpha link: %v %v", fi, err)
			}
		})
	}
}

// TestUpdateReposFollowsForcePushedUpstream covers the second dead end: a
// rewritten upstream that git pull --ff-only refuses on every update.
func TestUpdateReposFollowsForcePushedUpstream(t *testing.T) {
	const repoID = "github.com/example/rewritten"

	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origin := filepath.Join(dir, "origin")
	makeSkillRepo(t, origin, map[string]string{"alpha": "alpha v1"})
	redirectClones(t, "https://"+repoID, origin)

	storeDir := filepath.Join(dir, "store")
	storeRepo := RepoStorePath(storeDir, repoID)
	cloneStore(t, origin, storeRepo)
	oldCommit := gitHead(t, storeRepo)

	rewriteHistory(t, origin, "alpha", "alpha rewritten")
	newCommit := gitHead(t, origin)
	if oldCommit == newCommit {
		t.Fatal("rewriteHistory() did not change the origin commit")
	}

	lockPath := filepath.Join(dir, "lock.yaml")
	writeLock(t, lockPath, &types.LockFile{Skills: []types.InstalledSkill{{
		Name:      "alpha",
		Repo:      repoID,
		Commit:    oldCommit,
		SkillPath: filepath.Join(storeRepo, "alpha"),
	}}})

	cfg := &types.SkmConfig{
		Agents:   &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{{Repo: repoID, Skills: selectors("alpha")}},
	}
	if err := UpdateRepos(cfg, []string{repoID}, Options{StoreDir: storeDir, LockPath: lockPath}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	lf := readLock(t, lockPath)
	if len(lf.Skills) != 1 || lf.Skills[0].Commit != newCommit {
		t.Fatalf("lock after update = %+v, want alpha at %q", lf.Skills, newCommit)
	}
	if body := skillBody(t, storeRepo, "alpha"); !strings.Contains(body, "alpha rewritten") {
		t.Errorf("store SKILL.md = %q, want it to contain the rewritten content", body)
	}
}

// TestUpdateReposUpdatesEverySelectedRepo pins the multi-repo contract of the
// concurrent sync stage: every selected repo is pulled, and each skill's lock
// entry records the new commit of its own repo.
func TestUpdateReposUpdatesEverySelectedRepo(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	storeDir := filepath.Join(dir, "store")
	type repoFixture struct {
		repoID    string
		skill     string
		origin    string
		storeRepo string
		oldCommit string
		newCommit string
	}
	fixtures := []*repoFixture{
		{repoID: "github.com/example/first", skill: "alpha"},
		{repoID: "github.com/example/second", skill: "beta"},
	}
	for _, f := range fixtures {
		f.origin = filepath.Join(dir, f.skill+"-origin")
		makeSkillRepo(t, f.origin, map[string]string{f.skill: f.skill + " v1"})
		f.oldCommit = gitHead(t, f.origin)
		f.storeRepo = RepoStorePath(storeDir, f.repoID)
		cloneStore(t, f.origin, f.storeRepo)
		makeSkillRepoCommit(t, f.origin, map[string]string{f.skill: f.skill + " v2"})
		f.newCommit = gitHead(t, f.origin)
	}

	lockPath := filepath.Join(dir, "skills-lock.yaml")
	lf := &types.LockFile{}
	cfg := &types.SkmConfig{Agents: &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}}}
	var repos []string
	for _, f := range fixtures {
		lf.Skills = append(lf.Skills, types.InstalledSkill{
			Name:       f.skill,
			Repo:       f.repoID,
			Commit:     f.oldCommit,
			SkillPath:  filepath.Join(f.storeRepo, f.skill),
			SourcePath: f.skill,
			LinkedTo:   []string{filepath.Join(home, ".claude", "skills", f.skill)},
		})
		cfg.Packages = append(cfg.Packages, types.SkillPackageConfig{Repo: f.repoID, Skills: selectors(f.skill)})
		repos = append(repos, f.repoID)
	}
	writeLock(t, lockPath, lf)

	if err := UpdateRepos(cfg, repos, Options{StoreDir: storeDir, LockPath: lockPath}); err != nil {
		t.Fatalf("UpdateRepos() error: %v", err)
	}

	got := readLock(t, lockPath)
	byName := make(map[string]types.InstalledSkill, len(got.Skills))
	for _, skill := range got.Skills {
		byName[skill.Name] = skill
	}
	if len(got.Skills) != len(fixtures) {
		t.Fatalf("lock entries = %d, want %d: %+v", len(got.Skills), len(fixtures), got.Skills)
	}
	for _, f := range fixtures {
		entry, ok := byName[f.skill]
		if !ok {
			t.Fatalf("lock is missing %q: %+v", f.skill, got.Skills)
		}
		if entry.Commit != f.newCommit {
			t.Errorf("%s commit = %q, want %q", f.skill, entry.Commit, f.newCommit)
		}
		if body := skillBody(t, f.storeRepo, f.skill); !strings.Contains(body, f.skill+" v2") {
			t.Errorf("%s store SKILL.md = %q, want the updated content", f.skill, body)
		}
		link := filepath.Join(home, ".claude", "skills", f.skill)
		if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s link: %v %v", f.skill, fi, err)
		}
	}
}

// TestRunSyncsSharedRepoOnceForEveryPackage covers a config whose packages
// point at different subdirs of one repo, written in two different spellings:
// the sync stage collapses them to a single clone, and both packages still
// install from that one store copy.
func TestRunSyncsSharedRepoOnceForEveryPackage(t *testing.T) {
	const repoID = "github.com/example/shared"

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, origin, "init", "-b", "main")
	gitRun(t, origin, "config", "user.email", "test@example.com")
	gitRun(t, origin, "config", "user.name", "Test User")
	writeSkill(t, filepath.Join(origin, "packs", "one", "alpha"), "alpha")
	writeSkill(t, filepath.Join(origin, "packs", "two", "beta"), "beta")
	gitRun(t, origin, "add", ".")
	gitRun(t, origin, "commit", "-m", "initial skills")
	redirectClones(t, "https://"+repoID, origin)

	storeDir := filepath.Join(dir, "store")
	lockPath := filepath.Join(dir, "lock.yaml")
	cfg := &types.SkmConfig{
		Agents: &types.DefaultAgentsConfig{Default: []string{types.AgentClaude}},
		Packages: []types.SkillPackageConfig{
			{Repo: repoID + "/packs/one"},
			{Repo: "https://" + repoID + "/packs/two"},
		},
	}

	if err := Run(cfg, Options{StoreDir: storeDir, LockPath: lockPath}); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	got := readLock(t, lockPath)
	if len(got.Skills) != 2 {
		t.Fatalf("lock entries = %+v, want alpha and beta", got.Skills)
	}
	wantCommit := gitHead(t, origin)
	storeRepo := RepoStorePath(storeDir, repoID)
	for _, skill := range got.Skills {
		if skill.Repo != repoID {
			t.Errorf("%s repo = %q, want %q", skill.Name, skill.Repo, repoID)
		}
		if skill.Commit != wantCommit {
			t.Errorf("%s commit = %q, want %q", skill.Name, skill.Commit, wantCommit)
		}
		if !strings.HasPrefix(skill.SkillPath, storeRepo+string(filepath.Separator)) {
			t.Errorf("%s skill path = %q, want it inside the single store copy %q", skill.Name, skill.SkillPath, storeRepo)
		}
		link := filepath.Join(home, ".claude", "skills", skill.Name)
		if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s link: %v %v", skill.Name, fi, err)
		}
	}
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
