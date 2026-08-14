package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/foo/bar", "https://github.com/foo/bar"},
		{"https://github.com/foo/bar", "https://github.com/foo/bar"},
		{"http://github.com/foo/bar", "http://github.com/foo/bar"},
		{"git@github.com:foo/bar", "git@github.com:foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := repoURL(tt.input)
			if got != tt.want {
				t.Errorf("repoURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestCloneMaterialisesCheckout pins what the blobless partial clone must
// still deliver: a readable HEAD and the full working tree. A file:// origin
// honours the filter, a plain path origin makes git warn and ignore it, and
// both have to leave a usable clone behind.
func TestCloneMaterialisesCheckout(t *testing.T) {
	tests := []struct {
		name      string
		originURL func(origin string) string
	}{
		{
			name:      "file:// origin",
			originURL: func(origin string) string { return "file://" + filepath.ToSlash(origin) },
		},
		{
			name:      "local path origin",
			originURL: func(origin string) string { return filepath.ToSlash(origin) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			origin := filepath.Join(dir, "origin")
			originCommit := makeTestRepo(t, origin)
			// Clone turns a bare identifier into an https URL, so route that
			// URL to the local origin the way tests elsewhere do.
			redirectClones(t, "https://example.com/owner/repo", tt.originURL(origin))

			dest := filepath.Join(dir, "clone")
			if err := Clone("example.com/owner/repo", dest); err != nil {
				t.Fatalf("Clone() error: %v", err)
			}

			head, err := HeadCommit(dest)
			if err != nil {
				t.Fatalf("HeadCommit() error: %v", err)
			}
			if head != originCommit {
				t.Errorf("cloned HEAD = %q, want %q", head, originCommit)
			}
			body, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
			if err != nil {
				t.Fatalf("read the cloned working tree: %v", err)
			}
			if !strings.Contains(string(body), "skill body") {
				t.Errorf("cloned SKILL.md = %q, want the origin content", body)
			}
		})
	}
}

// makeTestRepo creates a one-commit repository at dir and returns its HEAD.
func makeTestRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("skill body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial")
	head, err := HeadCommit(dir)
	if err != nil {
		t.Fatal(err)
	}
	return head
}

// redirectClones points cloneURL at a local origin through git's insteadOf
// rewriting, so the test exercises the real clone path without network access.
func redirectClones(t *testing.T, cloneURL, origin string) {
	t.Helper()
	gitconfig := filepath.Join(t.TempDir(), "gitconfig")
	body := "[url \"" + origin + "\"]\n\tinsteadOf = " + cloneURL + "\n"
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
