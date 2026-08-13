package linker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/timonwong/skimi/internal/types"
)

// agentSkillsDir returns the agent-specific skills directory for the given agent.
func agentSkillsDir(agentName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	switch agentName {
	case types.AgentClaude:
		return filepath.Join(home, ".claude", "skills"), nil
	case types.AgentStandard:
		return filepath.Join(home, ".agents", "skills"), nil
	case types.AgentCodex:
		return filepath.Join(home, ".codex", "skills"), nil
	case types.AgentOpenClaw:
		return filepath.Join(home, ".openclaw", "skills"), nil
	case types.AgentPi:
		return filepath.Join(home, ".pi", "agent", "skills"), nil
	default:
		return "", fmt.Errorf("unknown agent: %s", agentName)
	}
}

// SkillLinkPath returns the destination path for a skill inside an agent's
// flat skills directory.
func SkillLinkPath(agentName, skillName string) (string, error) {
	base, err := agentSkillsDir(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, skillName), nil
}

// CreateLink installs a skill from srcPath at dstPath.
// It always creates a directory symlink.
func CreateLink(srcPath, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dstPath, err)
	}

	// Remove any existing link/dir at destination.
	if err := removeExisting(dstPath); err != nil {
		return err
	}

	return os.Symlink(srcPath, dstPath)
}

// RemoveLink removes the link at dstPath.
// Symlinks are removed directly; legacy hardlink trees are removed recursively.
func RemoveLink(dstPath string) error {
	return removeExisting(dstPath)
}

// IsManagedLink reports whether dstPath is a link skimi installed for the
// skill at srcPath: either a symlink resolving to srcPath, or a legacy
// hardlink tree whose SKILL.md still shares an inode with srcPath's SKILL.md.
// Anything else (user files, replaced directories, symlinks pointing
// elsewhere) is not skimi's to delete.
func IsManagedLink(dstPath, srcPath string) bool {
	fi, err := os.Lstat(dstPath)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(dstPath)
		if err != nil {
			return false
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(dstPath), target)
		}
		return filepath.Clean(target) == filepath.Clean(srcPath)
	}
	if !fi.IsDir() {
		return false
	}
	dstMD, err := os.Stat(filepath.Join(dstPath, "SKILL.md"))
	if err != nil {
		return false
	}
	srcMD, err := os.Stat(filepath.Join(srcPath, "SKILL.md"))
	if err != nil {
		return false
	}
	return os.SameFile(dstMD, srcMD)
}

// removeExisting removes dstPath whether it is a symlink, file, or directory.
func removeExisting(path string) error {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}
