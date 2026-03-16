package lock

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/timonwong/skimi/internal/fileutil"
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

	return fileutil.AtomicWrite(path, data)
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
