package lock

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/timonwong/skimi/internal/types"
	"gopkg.in/yaml.v3"
)

// Load reads and parses the lock file. If the file does not exist it returns
// an empty LockFile rather than an error.
func Load(path string) (*types.LockFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &types.LockFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lock file %s: %w", path, err)
	}

	var lf types.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lock file %s: %w", path, err)
	}
	return &lf, nil
}

// Save writes lf to path atomically, creating parent directories as needed.
func Save(path string, lf *types.LockFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lock file dir: %w", err)
	}

	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}

	return atomicWrite(path, data)
}

// FindByName returns the first InstalledSkill with the given name, or nil.
func FindByName(lf *types.LockFile, name string) *types.InstalledSkill {
	for i := range lf.Skills {
		if lf.Skills[i].Name == name {
			return &lf.Skills[i]
		}
	}
	return nil
}

// atomicWrite writes data to path via a temp file in the same directory.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".skimi-lock-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
