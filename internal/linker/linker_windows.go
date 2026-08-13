package linker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// createLinkFallback recovers from the one symlink failure every stock Windows
// install hands a non-elevated process: creating a symlink needs
// SeCreateSymbolicLinkPrivilege, granted only by Developer Mode or an elevated
// shell, and without it os.Symlink fails with ERROR_PRIVILEGE_NOT_HELD (1314).
// A directory junction resolves to the same place for readers and needs no
// privilege, so use one instead. Every other symlink failure is a real error
// and is returned untouched.
func createLinkFallback(srcPath, dstPath string, symlinkErr error) error {
	if !errors.Is(symlinkErr, windows.ERROR_PRIVILEGE_NOT_HELD) {
		return symlinkErr
	}
	if err := createJunction(srcPath, dstPath); err != nil {
		return fmt.Errorf("create junction %s (symlink needs Developer Mode or an elevated shell): %w", dstPath, err)
	}
	return nil
}

// isReparsePoint reports whether fi is a reparse point that Lstat could not
// classify further. Since Go 1.23 os.Lstat reports a directory junction as
// ModeIrregular with neither ModeDir nor ModeSymlink set, so IsManagedLink
// needs this to reach its SKILL.md ownership check for junctions.
func isReparsePoint(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeIrregular != 0
}

// createJunction creates a directory junction at dstPath pointing to srcPath.
// A junction is an empty directory carrying a mount point reparse point, so
// the directory is created first and the reparse data written into it; a
// failed write leaves nothing behind.
func createJunction(srcPath, dstPath string) error {
	target, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("resolve target %s: %w", srcPath, err)
	}
	data, err := encodeMountPointReparseData(target)
	if err != nil {
		return err
	}
	if err := os.Mkdir(dstPath, 0o755); err != nil {
		return fmt.Errorf("create junction dir %s: %w", dstPath, err)
	}
	if err := setReparsePoint(dstPath, data); err != nil {
		_ = os.Remove(dstPath)
		return err
	}
	return nil
}

// setReparsePoint attaches the reparse data to the existing directory at path.
func setReparsePoint(path string, data []byte) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("convert path %s: %w", path, err)
	}
	// FILE_FLAG_BACKUP_SEMANTICS is required to open a directory handle;
	// FILE_FLAG_OPEN_REPARSE_POINT keeps the open from being redirected.
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var returned uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT,
		&data[0], uint32(len(data)), nil, 0, &returned, nil); err != nil {
		return fmt.Errorf("set reparse point on %s: %w", path, err)
	}
	return nil
}
