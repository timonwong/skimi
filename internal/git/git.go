package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Clone clones repo into destPath.
func Clone(repo, destPath string) error {
	url := repoURL(repo)
	out, err := run("git", "clone", url, destPath)
	if err != nil {
		return fmt.Errorf("git clone %s: %w\n%s", repo, err, out)
	}
	return nil
}

// Pull runs git pull in repoPath.
func Pull(repoPath string) error {
	out, err := runIn(repoPath, "git", "pull", "--ff-only")
	if err != nil {
		return fmt.Errorf("git pull in %s: %w\n%s", repoPath, err, out)
	}
	return nil
}

// Fetch runs git fetch --all in repoPath.
func Fetch(repoPath string) error {
	out, err := runIn(repoPath, "git", "fetch", "--all")
	if err != nil {
		return fmt.Errorf("git fetch in %s: %w\n%s", repoPath, err, out)
	}
	return nil
}

// HeadCommit returns the full SHA of HEAD in repoPath.
func HeadCommit(repoPath string) (string, error) {
	out, err := runIn(repoPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w\n%s", repoPath, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// RevParse runs git rev-parse <ref> in repoPath and returns the result.
func RevParse(repoPath, ref string) (string, error) {
	out, err := runIn(repoPath, "git", "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s in %s: %w\n%s", ref, repoPath, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// Log returns a formatted log of commits between from and to in repoPath.
// Pass an empty string for from to get all commits up to to.
func Log(repoPath, from, to string) (string, error) {
	rangeArg := to
	if from != "" {
		rangeArg = from + ".." + to
	}
	out, err := runIn(repoPath, "git", "log", "--oneline", "--no-decorate", rangeArg)
	if err != nil {
		return "", fmt.Errorf("git log in %s: %w\n%s", repoPath, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// repoURL converts a short repo identifier (e.g. "github.com/foo/bar") to
// a git-cloneable HTTPS URL.
func repoURL(repo string) string {
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	return "https://" + repo
}

// run executes cmd and returns combined output.
func run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

// runIn executes cmd in the given directory and returns combined output.
func runIn(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}
