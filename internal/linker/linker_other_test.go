//go:build !windows

package linker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateLinkFallbackReturnsSymlinkError(t *testing.T) {
	sentinel := errors.New("symlink: permission denied")

	if got := createLinkFallback("/src", "/dst", sentinel); !errors.Is(got, sentinel) {
		t.Errorf("createLinkFallback() = %v, want the original symlink error %v", got, sentinel)
	}
}

func TestCreateLinkKeepsSymlinkErrorUnwrapped(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so os.Symlink would succeed")
	}
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A read-only parent makes os.Symlink fail while the destination itself
	// still stats as absent, so CreateLink reaches the symlink call.
	readOnly := filepath.Join(dir, "read-only")
	if err := os.Mkdir(readOnly, 0o555); err != nil {
		t.Fatal(err)
	}
	dstPath := filepath.Join(readOnly, "link")

	err := CreateLink(srcDir, dstPath)
	if err == nil {
		t.Fatal("CreateLink() = nil, want an error")
	}
	var linkErr *os.LinkError
	if !errors.As(err, &linkErr) || linkErr.Op != "symlink" {
		t.Errorf("CreateLink() = %v, want the raw *os.LinkError from os.Symlink", err)
	}
}

func TestIsReparsePoint(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "symlink")
	if err := os.Symlink(subDir, symlink); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"directory", subDir},
		{"file", file},
		{"symlink", symlink},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi, err := os.Lstat(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if isReparsePoint(fi) {
				t.Errorf("isReparsePoint(%q) = true, want false outside Windows", tt.path)
			}
		})
	}
}
