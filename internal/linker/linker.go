package linker

import (
	"fmt"
	"io"
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

// useHardlink reports whether an agent requires hardlinks instead of symlinks.
func useHardlink(agentName string) bool {
	return agentName == types.AgentStandard || agentName == types.AgentOpenClaw
}

// SkillLinkPath returns the destination path for a skill inside an agent's
// skills directory, taking target_dir into account.
func SkillLinkPath(agentName, targetDir, skillName string) (string, error) {
	base, err := agentSkillsDir(agentName)
	if err != nil {
		return "", err
	}
	if targetDir != "" {
		return filepath.Join(base, targetDir, skillName), nil
	}
	return filepath.Join(base, skillName), nil
}

// CreateLink installs a skill from srcPath at dstPath.
// For agents that use hardlinks, it mirrors the directory tree with hardlinks;
// otherwise it creates a single symlink.
func CreateLink(srcPath, dstPath string, agentName string) error {
	hard := useHardlink(agentName)

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dstPath, err)
	}

	// Remove any existing link/dir at destination.
	if err := removeExisting(dstPath); err != nil {
		return err
	}

	if hard {
		return hardlinkTree(srcPath, dstPath)
	}
	return os.Symlink(srcPath, dstPath)
}

// RemoveLink removes the link at dstPath.
// Symlinks are removed directly; directories (hardlink trees) are removed recursively.
func RemoveLink(dstPath string) error {
	return removeExisting(dstPath)
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

// hardlinkTree recursively mirrors srcDir into dstDir using hardlinks for
// regular files and creating matching subdirectories.
func hardlinkTree(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dstDir, err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", srcDir, err)
	}

	for _, e := range entries {
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())

		if e.IsDir() {
			if err := hardlinkTree(src, dst); err != nil {
				return err
			}
			continue
		}

		if err := hardlinkFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// hardlinkFile creates a hardlink at dst pointing to src.
// Falls back to a file copy if the cross-device link error occurs.
func hardlinkFile(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	// Cross-device or unsupported: fall back to copy.
	return copyFile(src, dst)
}

// copyFile copies src to dst preserving the file mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return nil
}
