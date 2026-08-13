package types

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// SkmConfig represents the top-level configuration file structure (skills.yaml).
type SkmConfig struct {
	Agents   *DefaultAgentsConfig `yaml:"agents,omitempty"`
	Packages []SkillPackageConfig `yaml:"packages"`
}

// DefaultAgentsConfig specifies the default agent list applied when a package
// does not define its own agents section.
type DefaultAgentsConfig struct {
	Default []string `yaml:"default,omitempty"`
}

// AgentsConfig filters which agents receive the skills from a specific package.
type AgentsConfig struct {
	Includes []string `yaml:"includes,omitempty"`
	Excludes []string `yaml:"excludes,omitempty"`
}

// SkillPackageConfig describes a single skill source (remote repo or local path).
type SkillPackageConfig struct {
	Repo      string          `yaml:"repo,omitempty"`
	LocalPath string          `yaml:"local_path,omitempty"`
	TargetDir string          `yaml:"target_dir,omitempty"` // deprecated; ignored
	Skills    []SkillSelector `yaml:"skills,omitempty"`     // empty means all detected skills
	Agents    *AgentsConfig   `yaml:"agents,omitempty"`
}

// SkillSelector selects a skill by declared name or by path relative to the
// effective scan root.
type SkillSelector struct {
	Name string
	Path string
}

func (s *SkillSelector) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return fmt.Errorf("skill selector must be a name or path mapping")
		}
		s.Name = node.Value
		return nil
	case yaml.MappingNode:
		if len(node.Content) != 2 || node.Content[0].Value != "path" {
			return fmt.Errorf("skill selector mapping must contain only path")
		}
		s.Path = node.Content[1].Value
		return nil
	default:
		return fmt.Errorf("skill selector must be a name or path mapping")
	}
}

func (s SkillSelector) MarshalYAML() (any, error) {
	if s.Path != "" {
		return map[string]string{"path": s.Path}, nil
	}
	return s.Name, nil
}

// LockFile represents the lock file (skills-lock.yaml) that records installed state.
type LockFile struct {
	Version int              `yaml:"version,omitempty"`
	Skills  []InstalledSkill `yaml:"skills"`
}

// InstalledSkill records one installed skill and where it is linked.
type InstalledSkill struct {
	Name       string   `yaml:"name"`
	Source     string   `yaml:"source,omitempty"` // canonical configured source identity
	Repo       string   `yaml:"repo,omitempty"`
	LocalPath  string   `yaml:"local_path,omitempty"`
	Commit     string   `yaml:"commit,omitempty"`
	SkillPath  string   `yaml:"skill_path"`           // absolute path in store or local_path
	SourcePath string   `yaml:"path,omitempty"`       // relative to effective scan root
	TargetDir  string   `yaml:"target_dir,omitempty"` // v1 compatibility only
	LinkedTo   []string `yaml:"linked_to"`            // absolute link paths created for this skill
}

// DetectedSkill is returned by the detect package for each SKILL.md found.
type DetectedSkill struct {
	Name        string
	Description string
	SkillPath   string // absolute path to the directory containing SKILL.md
	SourcePath  string // slash-separated path relative to effective scan root
}

// Known agent names supported by skimi.
const (
	AgentClaude   = "claude"
	AgentStandard = "standard"
	AgentCodex    = "codex"
	AgentOpenClaw = "openclaw"
	AgentPi       = "pi"
)

// AllAgents lists every agent name skimi knows about.
var AllAgents = []string{
	AgentClaude,
	AgentStandard,
	AgentCodex,
	AgentOpenClaw,
	AgentPi,
}
