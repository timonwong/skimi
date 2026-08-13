//go:build !windows

package linker

import "os"

// createLinkFallback has nothing to fall back to outside Windows: os.Symlink
// is the only link mechanism, so its error stands. The junction fallback in
// linker_windows.go exists only because stock Windows refuses symlinks.
func createLinkFallback(_, _ string, symlinkErr error) error {
	return symlinkErr
}

// isReparsePoint is always false outside Windows, where os.Lstat reports
// everything as a symlink, a directory, or a file.
func isReparsePoint(os.FileInfo) bool {
	return false
}
