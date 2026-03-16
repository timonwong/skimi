package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/timonwong/skimi/internal/detect"
	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/linker"
	"github.com/timonwong/skimi/internal/lock"
	"github.com/timonwong/skimi/internal/types"
)

// Options controls the behaviour of Run.
type Options struct {
	StoreDir string // root directory for cloned repos
	LockPath string // path to the lock file
	DryRun   bool   // print what would be done without making changes
}

// Run installs all packages declared in cfg and updates the lock file.
func Run(cfg *types.SkmConfig, opts Options) error {
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return fmt.Errorf("load lock file: %w", err)
	}

	// Build a set of currently installed skill names for stale-link detection.
	oldSkills := make(map[string]types.InstalledSkill, len(lf.Skills))
	for _, s := range lf.Skills {
		oldSkills[s.Name] = s
	}

	defaultAgents := resolveDefaultAgents(cfg)

	var newSkills []types.InstalledSkill

	for _, pkg := range cfg.Packages {
		installed, err := installPackage(pkg, defaultAgents, opts)
		if err != nil {
			return err
		}
		newSkills = append(newSkills, installed...)
	}

	// Remove stale links that are no longer declared.
	newSkillNames := make(map[string]struct{}, len(newSkills))
	for _, s := range newSkills {
		newSkillNames[s.Name] = struct{}{}
	}

	for name, old := range oldSkills {
		if _, ok := newSkillNames[name]; ok {
			continue
		}
		fmt.Printf("Removing stale skill %q\n", name)
		if !opts.DryRun {
			for _, link := range old.LinkedTo {
				if err := linker.RemoveLink(link); err != nil {
					fmt.Fprintf(os.Stderr, "warning: remove link %s: %v\n", link, err)
				}
			}
		}
	}

	if !opts.DryRun {
		newLF := &types.LockFile{Skills: newSkills}
		if err := lock.Save(opts.LockPath, newLF); err != nil {
			return fmt.Errorf("save lock file: %w", err)
		}
	}

	return nil
}

// installPackage processes a single SkillPackageConfig and returns the
// InstalledSkill entries it produced.
func installPackage(pkg types.SkillPackageConfig, defaultAgents []string, opts Options) ([]types.InstalledSkill, error) {
	var sourceDir string
	var repo, localPath string

	switch {
	case pkg.Repo != "":
		repo = pkg.Repo
		dest := RepoStorePath(opts.StoreDir, pkg.Repo)
		if err := ensureRepo(pkg.Repo, dest); err != nil {
			return nil, err
		}
		sourceDir = dest

	case pkg.LocalPath != "":
		localPath = pkg.LocalPath
		expanded, err := ExpandPath(pkg.LocalPath)
		if err != nil {
			return nil, err
		}
		sourceDir = expanded

	default:
		return nil, fmt.Errorf("package has neither repo nor local_path")
	}

	// Detect skills in the source directory.
	detected, err := detect.Scan(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("detect skills in %s: %w", sourceDir, err)
	}

	// Filter to the requested skills if specified.
	if len(pkg.Skills) > 0 {
		detected = filterSkills(detected, pkg.Skills)
	}

	// Determine commit for repo packages.
	var commit string
	if repo != "" {
		dest := RepoStorePath(opts.StoreDir, repo)
		commit, _ = git.HeadCommit(dest)
	}

	// Determine agent list for this package.
	agents := resolvePackageAgents(pkg, defaultAgents)

	var installed []types.InstalledSkill

	for _, skill := range detected {
		links, err := linkSkill(skill, agents, pkg.TargetDir, opts.DryRun)
		if err != nil {
			return nil, err
		}

		entry := types.InstalledSkill{
			Name:      skill.Name,
			SkillPath: skill.SkillPath,
			TargetDir: pkg.TargetDir,
			LinkedTo:  links,
		}
		if repo != "" {
			entry.Repo = repo
			entry.Commit = commit
		} else {
			entry.LocalPath = localPath
		}

		installed = append(installed, entry)
		fmt.Printf("Installed skill %q → %s\n", skill.Name, strings.Join(links, ", "))
	}

	return installed, nil
}

// linkSkill creates links for skill in each agent's skills directory.
func linkSkill(skill types.DetectedSkill, agents []string, targetDir string, dryRun bool) ([]string, error) {
	var links []string
	for _, agent := range agents {
		dstPath, err := linker.SkillLinkPath(agent, targetDir, skill.Name)
		if err != nil {
			return nil, err
		}
		if dryRun {
			fmt.Printf("  [dry-run] link %s → %s\n", skill.SkillPath, dstPath)
		} else {
			if err := linker.CreateLink(skill.SkillPath, dstPath, agent); err != nil {
				return nil, fmt.Errorf("create link for %s in agent %s: %w", skill.Name, agent, err)
			}
		}
		links = append(links, dstPath)
	}
	return links, nil
}

// ensureRepo clones the repo if dest does not exist, or pulls if it does.
func ensureRepo(repo, dest string) error {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		fmt.Printf("Cloning %s ...\n", repo)
		return git.Clone(repo, dest)
	}
	fmt.Printf("Updating %s ...\n", repo)
	return git.Pull(dest)
}

// RepoStorePath converts a repo identifier to its path inside the store dir.
// e.g. "github.com/foo/bar" → "<store>/github.com/foo/bar"
func RepoStorePath(storeDir, repo string) string {
	// Strip protocol prefix if present.
	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimPrefix(repo, "http://")
	repo = strings.TrimPrefix(repo, "git@")
	repo = strings.ReplaceAll(repo, ":", "/")
	return filepath.Join(storeDir, repo)
}

// ExpandPath expands ~ and returns an absolute path.
func ExpandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}

// filterSkills returns only the detected skills whose names appear in want.
func filterSkills(all []types.DetectedSkill, want []string) []types.DetectedSkill {
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	var out []types.DetectedSkill
	for _, s := range all {
		if _, ok := wantSet[s.Name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// resolveDefaultAgents returns the default agent list from cfg, falling back
// to all known agents if none is configured.
func resolveDefaultAgents(cfg *types.SkmConfig) []string {
	if cfg.Agents != nil && len(cfg.Agents.Default) > 0 {
		return cfg.Agents.Default
	}
	return types.AllAgents
}

// resolvePackageAgents computes the effective agent list for a package,
// applying includes/excludes on top of the defaults.
func resolvePackageAgents(pkg types.SkillPackageConfig, defaultAgents []string) []string {
	if pkg.Agents == nil {
		return defaultAgents
	}

	base := defaultAgents
	if len(pkg.Agents.Includes) > 0 {
		// Includes overrides the default list entirely.
		base = pkg.Agents.Includes
	}

	if len(pkg.Agents.Excludes) == 0 {
		return base
	}

	excludeSet := make(map[string]struct{}, len(pkg.Agents.Excludes))
	for _, e := range pkg.Agents.Excludes {
		excludeSet[e] = struct{}{}
	}

	var out []string
	for _, a := range base {
		if _, excluded := excludeSet[a]; !excluded {
			out = append(out, a)
		}
	}
	return out
}
