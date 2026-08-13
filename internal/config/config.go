package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse config %s: multiple YAML documents are not supported", path)
		}
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config %s: %w", path, err)
	}
	return &cfg, nil
}

// Validate checks the complete configuration before installation mutates the
// filesystem. Source-dependent selector checks happen after skill discovery.
func Validate(cfg *types.SkmConfig) error {
	if cfg.Agents != nil {
		if err := validateAgents("agents.default", cfg.Agents.Default); err != nil {
			return err
		}
	}
	for i, pkg := range cfg.Packages {
		prefix := fmt.Sprintf("packages[%d]", i)
		if (pkg.Repo == "") == (pkg.LocalPath == "") {
			return fmt.Errorf("%s must set exactly one of repo or local_path", prefix)
		}
		if pkg.Agents != nil {
			if err := validateAgents(prefix+".agents.includes", pkg.Agents.Includes); err != nil {
				return err
			}
			if err := validateAgents(prefix+".agents.excludes", pkg.Agents.Excludes); err != nil {
				return err
			}
		}
		for j, selector := range pkg.Skills {
			if (selector.Name == "") == (selector.Path == "") {
				return fmt.Errorf("%s.skills[%d] must set exactly one of name or path", prefix, j)
			}
			if selector.Name != "" && strings.TrimSpace(selector.Name) != selector.Name {
				return fmt.Errorf("%s.skills[%d] name must not contain surrounding whitespace", prefix, j)
			}
			if selector.Path != "" {
				clean := filepath.ToSlash(filepath.Clean(selector.Path))
				if filepath.IsAbs(selector.Path) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != selector.Path {
					return fmt.Errorf("%s.skills[%d] path must be a clean relative slash-separated path", prefix, j)
				}
			}
		}
	}
	return nil
}

func validateAgents(field string, agents []string) error {
	seen := make(map[string]struct{}, len(agents))
	for _, agent := range agents {
		if !slices.Contains(types.AllAgents, agent) {
			return fmt.Errorf("%s contains unknown agent %q", field, agent)
		}
		if _, ok := seen[agent]; ok {
			return fmt.Errorf("%s contains duplicate agent %q", field, agent)
		}
		seen[agent] = struct{}{}
	}
	return nil
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
