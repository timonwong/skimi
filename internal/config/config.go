package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/timonwong/skimi/internal/fileutil"
	"github.com/timonwong/skimi/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	defaultConfigDir  = ".config/skimi"
	defaultConfigFile = "skills.yaml"
	defaultLockFile   = "skills-lock.yaml"
	defaultStoreBase  = ".local/share/skimi/skills"
)

// Paths holds resolved filesystem paths used by skimi.
type Paths struct {
	ConfigFile string
	LockFile   string
	StoreDir   string
}

// DefaultPaths returns the default Paths based on the user's home directory.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}
	return Paths{
		ConfigFile: filepath.Join(home, defaultConfigDir, defaultConfigFile),
		LockFile:   filepath.Join(home, defaultConfigDir, defaultLockFile),
		StoreDir:   filepath.Join(home, defaultStoreBase),
	}, nil
}

// Load reads and parses the skills.yaml config file. If the file does not
// exist it returns an empty config rather than an error.
func Load(path string) (*types.SkmConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &types.SkmConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg types.SkmConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes cfg to path, creating parent directories as needed.
// The file is written atomically via a temporary file + rename.
func Save(path string, cfg *types.SkmConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return fileutil.AtomicWrite(path, data)
}
