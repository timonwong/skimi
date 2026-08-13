package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

			got, isRemote, err := resolveSource(repoID, storeDir, false)
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

// TestResolveSourceDryRun pins the read-only contract of an interactive dry
// run: an existing store copy is previewed without moving its working tree,
// and a missing one stops before cloning instead of mutating the store.
func TestResolveSourceDryRun(t *testing.T) {
	const repoID = "github.com/example/dryrun"

	t.Run("missing store copy stops before cloning", func(t *testing.T) {
		dir := t.TempDir()
		origin := filepath.Join(dir, "origin")
		initSkillRepo(t, origin, "alpha")
		storeDir := filepath.Join(dir, "store")
		redirectClones(t, "https://"+repoID, origin)

		_, isRemote, err := resolveSource(repoID, storeDir, true)
		if !errors.Is(err, errDryRunNotCloned) {
			t.Fatalf("resolveSource() error = %v, want errDryRunNotCloned", err)
		}
		if !isRemote {
			t.Error("resolveSource() isRemote = false, want true")
		}
		if _, statErr := os.Stat(installer.RepoStorePath(storeDir, repoID)); !os.IsNotExist(statErr) {
			t.Fatalf("dry run created a store copy: %v", statErr)
		}
	})

	t.Run("cached store copy is previewed without moving it", func(t *testing.T) {
		dir := t.TempDir()
		origin := filepath.Join(dir, "origin")
		initSkillRepo(t, origin, "alpha")
		storeDir := filepath.Join(dir, "store")
		storeRepo := installer.RepoStorePath(storeDir, repoID)
		gitRun(t, dir, "clone", origin, storeRepo)
		oldHead := gitHead(t, storeRepo)

		// Advance origin so a pull would move the store working tree.
		body := []byte("---\nname: alpha\ndescription: test skill\n---\n\nv2\n")
		if err := os.WriteFile(filepath.Join(origin, "alpha", "SKILL.md"), body, 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, origin, "commit", "-am", "advance")
		redirectClones(t, "https://"+repoID, origin)

		got, isRemote, err := resolveSource(repoID, storeDir, true)
		if err != nil {
			t.Fatalf("resolveSource() error: %v", err)
		}
		if !isRemote || got != storeRepo {
			t.Fatalf("resolveSource() = %q, %v, want %q, true", got, isRemote, storeRepo)
		}
		if head := gitHead(t, storeRepo); head != oldHead {
			t.Fatalf("dry run moved the store checkout: %q -> %q", oldHead, head)
		}
	})
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v\n%s", dir, err, out)
	}
	return strings.TrimSpace(string(out))
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
