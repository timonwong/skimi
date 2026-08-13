package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/installer"
)

// TestResolveSourceSyncPolicy pins how the interactive path treats a failed
// sync: it repairs a store copy git cannot use, keeps browsing a cached copy
// when the remote is out of reach, and only fails when neither is available.
func TestResolveSourceSyncPolicy(t *testing.T) {
	const repoID = "github.com/example/interactive"

	tests := []struct {
		name string
		// prepare leaves the store copy in the state under test and reports the
		// URL https://<repoID> resolves to, so no case touches the network.
		prepare func(t *testing.T, dir, origin, storeRepo string) (cloneTarget string)
		wantErr bool
	}{
		{
			name: "broken store copy is repaired",
			prepare: func(t *testing.T, _, origin, storeRepo string) string {
				if err := os.MkdirAll(storeRepo, 0o755); err != nil {
					t.Fatal(err)
				}
				return origin
			},
		},
		{
			name: "unreachable remote falls back to the cached copy",
			prepare: func(t *testing.T, dir, origin, storeRepo string) string {
				gitRun(t, dir, "clone", origin, storeRepo)
				missing := filepath.Join(dir, "missing-origin")
				gitRun(t, storeRepo, "remote", "set-url", "origin", missing)
				return missing
			},
		},
		{
			name: "unreachable remote without a cached copy fails",
			prepare: func(t *testing.T, dir, _, _ string) string {
				return filepath.Join(dir, "missing-origin")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			origin := filepath.Join(dir, "origin")
			initSkillRepo(t, origin, "alpha")

			storeDir := filepath.Join(dir, "store")
			storeRepo := installer.RepoStorePath(storeDir, repoID)
			redirectClones(t, "https://"+repoID, tt.prepare(t, dir, origin, storeRepo))

			got, isRemote, err := resolveSource(repoID, storeDir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("resolveSource() expected an error when nothing usable is left")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSource() error: %v", err)
			}
			if !isRemote {
				t.Error("resolveSource() isRemote = false, want true for a remote source")
			}
			if got != storeRepo {
				t.Errorf("resolveSource() dir = %q, want %q", got, storeRepo)
			}
			if !git.IsRepoRoot(got) {
				t.Fatalf("%s is not a usable clone after resolveSource()", got)
			}
			if _, err := os.Stat(filepath.Join(got, "alpha", "SKILL.md")); err != nil {
				t.Fatalf("resolved source is missing the skill: %v", err)
			}
		})
	}
}

func initSkillRepo(t *testing.T, dir, skill string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, skill), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\nname: " + skill + "\ndescription: test skill\n---\n")
	if err := os.WriteFile(filepath.Join(dir, skill, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test User")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "add skills")
}

// redirectClones points cloneURL at a local path through git's insteadOf
// rewriting, so tests exercise the real clone path without network access.
func redirectClones(t *testing.T, cloneURL, target string) {
	t.Helper()
	gitconfig := filepath.Join(t.TempDir(), "gitconfig")
	body := "[url \"" + filepath.ToSlash(target) + "\"]\n\tinsteadOf = " + cloneURL + "\n"
	if err := os.WriteFile(gitconfig, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
