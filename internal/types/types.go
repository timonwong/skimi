package types

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
	Repo      string        `yaml:"repo,omitempty"`
	LocalPath string        `yaml:"local_path,omitempty"`
	TargetDir string        `yaml:"target_dir,omitempty"` // optional sub-directory under agent skills dir
	Skills    []string      `yaml:"skills,omitempty"`     // empty means all detected skills
	Agents    *AgentsConfig `yaml:"agents,omitempty"`
}

// LockFile represents the lock file (skills-lock.yaml) that records installed state.
type LockFile struct {
	Skills []InstalledSkill `yaml:"skills"`
}

// InstalledSkill records one installed skill and where it is linked.
type InstalledSkill struct {
	Name      string   `yaml:"name"`
	Repo      string   `yaml:"repo,omitempty"`
	LocalPath string   `yaml:"local_path,omitempty"`
	Commit    string   `yaml:"commit,omitempty"`
	SkillPath string   `yaml:"skill_path"` // absolute path in store or local_path
	TargetDir string   `yaml:"target_dir,omitempty"`
	LinkedTo  []string `yaml:"linked_to"` // absolute link paths created for this skill
}

// DetectedSkill is returned by the detect package for each SKILL.md found.
type DetectedSkill struct {
	Name        string
	Description string
	SkillPath   string // absolute path to the directory containing SKILL.md
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
