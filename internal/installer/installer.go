package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/timonwong/skimi/internal/config"
	"github.com/timonwong/skimi/internal/detect"
	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/linker"
	"github.com/timonwong/skimi/internal/lock"
	"github.com/timonwong/skimi/internal/source"
	"github.com/timonwong/skimi/internal/types"
	"github.com/timonwong/skimi/internal/ui"
)

// Options controls the behaviour of Run.
type Options struct {
	StoreDir string // root directory for cloned repos
	LockPath string // path to the lock file
	DryRun   bool   // print what would be done without making changes
	Verbose  bool   // reserved for additional installation detail
}

// Run installs all packages declared in cfg and updates the lock file.
func Run(cfg *types.SkmConfig, opts Options) error {
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return fmt.Errorf("load lock file: %w", err)
	}

	defaultAgents := resolveDefaultAgents(cfg)
	var candidates []installCandidate
	for i, pkg := range cfg.Packages {
		if i > 0 {
			fmt.Println()
		}
		prepared, err := preparePackage(pkg, defaultAgents, opts, true)
		if err != nil {
			return err
		}
		candidates = append(candidates, prepared...)
	}
	return applyPlan(lf, resolveCollisions(candidates), opts)
}

// UpdateRepos updates the selected remote repos and preserves unrelated lock entries.
func UpdateRepos(cfg *types.SkmConfig, repos []string, opts Options) error {
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return fmt.Errorf("load lock file: %w", err)
	}

	for i, repo := range repos {
		if i > 0 {
			fmt.Println()
		}

		pkgs, err := packagesForRepo(cfg, repo)
		if err != nil {
			return err
		}
		oldCommit := lockedRepoCommit(lf, repo)

		dest := RepoStorePath(opts.StoreDir, repo)
		fmt.Println(ui.Blue.Render("Pulling " + repo + " ..."))
		if err := ensureRepo(pkgs[0].cloneURL, dest); err != nil {
			return err
		}
		newCommit, err := git.HeadCommit(dest)
		if err != nil {
			return err
		}

		if oldCommit == newCommit {
			commit := shortCommit(newCommit)
			if commit == "" {
				commit = "unknown"
			}
			fmt.Println(ui.Green.Render("  Already up to date (" + commit + ")"))
			continue
		}

		fmt.Printf("  Updated %s -> %s\n", ui.Red.Render(shortCommit(oldCommit)), ui.Green.Render(shortCommit(newCommit)))
		if oldCommit != "" {
			log, err := git.Log(dest, oldCommit, newCommit)
			if err == nil && log != "" {
				for _, line := range strings.Split(log, "\n") {
					fmt.Println(ui.Dim.Render("    " + line))
				}
			}
		}
	}
	defaultAgents := resolveDefaultAgents(cfg)
	var candidates []installCandidate
	changedRepos := make(map[string]struct{}, len(repos))
	for _, repo := range repos {
		changedRepos[repo] = struct{}{}
	}
	declaredSources := make(map[string]struct{}, len(cfg.Packages))
	for _, pkg := range cfg.Packages {
		if pkg.Repo != "" {
			parsed, err := source.Parse(pkg.Repo)
			if err != nil {
				return err
			}
			declaredSources["repo:"+parsed.Repo] = struct{}{}
		} else {
			declaredSources["local:"+pkg.LocalPath] = struct{}{}
		}
	}
	for _, skill := range lf.Skills {
		key := "repo:" + skill.Repo
		if skill.Repo == "" {
			key = "local:" + skill.LocalPath
		}
		if _, declared := declaredSources[key]; !declared {
			candidates = append(candidates, installCandidate{entry: skill, agents: agentLabels(skill.LinkedTo), preserved: true})
		}
	}
	for _, pkg := range cfg.Packages {
		var repoID string
		if pkg.Repo != "" {
			parsed, err := source.Parse(pkg.Repo)
			if err != nil {
				return err
			}
			repoID = parsed.Repo
		}
		if _, isChanged := changedRepos[repoID]; isChanged {
			prepared, err := preparePackage(pkg, defaultAgents, opts, false)
			if err != nil {
				return err
			}
			candidates = append(candidates, prepared...)
			continue
		}
		for _, skill := range lf.Skills {
			if (repoID != "" && skill.Repo == repoID) || (pkg.LocalPath != "" && skill.LocalPath == pkg.LocalPath) {
				candidates = append(candidates, installCandidate{entry: skill, agents: agentLabels(skill.LinkedTo), preserved: true})
			}
		}
	}
	return applyPlan(lf, resolveCollisions(candidates), opts)
}

func agentLabels(links []string) []string {
	out := make([]string, len(links))
	for i, link := range links {
		slash := filepath.ToSlash(link)
		switch {
		case strings.Contains(slash, "/.claude/skills/"):
			out[i] = types.AgentClaude
		case strings.Contains(slash, "/.agents/skills/"):
			out[i] = types.AgentStandard
		case strings.Contains(slash, "/.codex/skills/"):
			out[i] = types.AgentCodex
		case strings.Contains(slash, "/.openclaw/skills/"):
			out[i] = types.AgentOpenClaw
		case strings.Contains(slash, "/.pi/agent/skills/"):
			out[i] = types.AgentPi
		default:
			out[i] = "unknown"
		}
	}
	return out
}

type repoPackage struct {
	config   types.SkillPackageConfig
	cloneURL string
}

func packagesForRepo(cfg *types.SkmConfig, repo string) ([]repoPackage, error) {
	var out []repoPackage
	for _, pkg := range cfg.Packages {
		if pkg.Repo == "" {
			continue
		}
		parsed, err := source.Parse(pkg.Repo)
		if err != nil {
			return nil, fmt.Errorf("parse repo %q: %w", pkg.Repo, err)
		}
		if parsed.Kind != source.SourceRemote {
			continue
		}
		if parsed.Repo == repo {
			out = append(out, repoPackage{
				config:   pkg,
				cloneURL: parsed.GetCloneURL(),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("repo %q is not declared in config", repo)
	}
	return out, nil
}

func lockedRepoCommit(lf *types.LockFile, repo string) string {
	for _, skill := range lf.Skills {
		if skill.Repo == repo {
			return skill.Commit
		}
	}
	return ""
}

func shortCommit(commit string) string {
	if len(commit) <= 8 {
		return commit
	}
	return commit[:8]
}

type installCandidate struct {
	entry     types.InstalledSkill
	agents    []string
	preserved bool
}

var validSkillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// preparePackage resolves and validates a package without changing agent skill
// directories or the lock file.
func preparePackage(pkg types.SkillPackageConfig, defaultAgents []string, opts Options, syncRemote bool) ([]installCandidate, error) {
	var sourceDir string
	var repo, localPath, sourceIdentity string

	switch {
	case pkg.Repo != "":
		// Parse the repo to handle shorthand formats like "owner/repo"
		parsed, err := source.Parse(pkg.Repo)
		if err != nil {
			return nil, fmt.Errorf("parse repo %q: %w", pkg.Repo, err)
		}
		if parsed.Kind != source.SourceRemote {
			return nil, fmt.Errorf("repo %q resolved to local path, use local_path instead", pkg.Repo)
		}

		repo = parsed.Repo
		dest := RepoStorePath(opts.StoreDir, parsed.Repo)

		_, statErr := os.Stat(dest)
		if os.IsNotExist(statErr) {
			fmt.Println(ui.Blue.Render("Using " + repo))
		} else {
			fmt.Println(ui.Blue.Render("Using existing " + repo))
		}

		if syncRemote {
			if err := ensureRepo(parsed.GetCloneURL(), dest); err != nil {
				return nil, err
			}
		}

		// Apply subdir if specified in the source
		sourceDir = dest
		if parsed.Subdir != "" {
			sourceDir = filepath.Join(dest, parsed.Subdir)
		}
		sourceIdentity = parsed.Repo
		if parsed.Subdir != "" {
			sourceIdentity += "/" + filepath.ToSlash(filepath.Clean(parsed.Subdir))
		}

	case pkg.LocalPath != "":
		localPath = pkg.LocalPath
		expanded, err := ExpandPath(pkg.LocalPath)
		if err != nil {
			return nil, err
		}
		sourceDir = expanded
		sourceIdentity = filepath.Clean(expanded)
		fmt.Println(ui.Blue.Render("Using local path " + pkg.LocalPath))

	default:
		return nil, fmt.Errorf("package has neither repo nor local_path")
	}

	// Detect skills in the source directory.
	detected, err := detect.Scan(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("detect skills in %s: %w", sourceDir, err)
	}

	for _, skill := range detected {
		if len(skill.Name) > 64 || !validSkillName.MatchString(skill.Name) {
			return nil, fmt.Errorf("invalid skill name %q at %s: use lowercase letters, numbers, and hyphens (maximum 64 characters)", skill.Name, skill.SkillPath)
		}
	}
	sort.SliceStable(detected, func(i, j int) bool { return detected[i].SourcePath < detected[j].SourcePath })

	if len(pkg.Skills) > 0 {
		detected, err = selectSkills(detected, pkg.Skills)
		if err != nil {
			return nil, fmt.Errorf("select skills from %s: %w", sourceIdentity, err)
		}
	}

	skillNames := make([]string, len(detected))
	for i, s := range detected {
		skillNames[i] = s.Name
	}
	fmt.Println(ui.Dim.Render("  Found skills: " + strings.Join(skillNames, ", ")))

	// Determine commit for repo packages.
	var commit string
	if repo != "" {
		commit, _ = git.HeadCommit(sourceDir)
	}

	// Determine agent list for this package.
	agents := resolvePackageAgents(pkg, defaultAgents)
	if pkg.TargetDir != "" {
		fmt.Fprintf(os.Stderr, "warning: target_dir %q is deprecated and ignored; skills are installed flat\n", pkg.TargetDir)
	}

	var candidates []installCandidate

	for _, skill := range detected {
		links := make([]string, 0, len(agents))
		for _, agent := range agents {
			dstPath, err := linker.SkillLinkPath(agent, skill.Name)
			if err != nil {
				return nil, err
			}
			links = append(links, dstPath)
		}

		entry := types.InstalledSkill{
			Name:       skill.Name,
			Source:     sourceIdentity,
			SkillPath:  skill.SkillPath,
			SourcePath: skill.SourcePath,
			LinkedTo:   links,
		}
		if repo != "" {
			entry.Repo = repo
			entry.Commit = commit
		} else {
			entry.LocalPath = localPath
		}

		candidates = append(candidates, installCandidate{entry: entry, agents: append([]string(nil), agents...)})
	}

	return candidates, nil
}

func resolveCollisions(candidates []installCandidate) []installCandidate {
	winners := make(map[string]int, len(candidates))
	var out []installCandidate
	for _, candidate := range candidates {
		if index, ok := winners[candidate.entry.Name]; ok {
			loser := out[index]
			fmt.Fprintf(os.Stderr, "warning: skill %q from %s/%s is overridden by %s/%s (last wins)\n",
				candidate.entry.Name, loser.entry.Source, loser.entry.SourcePath, candidate.entry.Source, candidate.entry.SourcePath)
			out[index] = candidate
			continue
		}
		winners[candidate.entry.Name] = len(out)
		out = append(out, candidate)
	}
	return out
}

type replacedLink struct {
	path   string
	backup string
}

func applyPlan(old *types.LockFile, winners []installCandidate, opts Options) error {
	owned := make(map[string]string)
	for _, skill := range old.Skills {
		for _, link := range skill.LinkedTo {
			owned[link] = skill.SkillPath
		}
	}

	for _, candidate := range winners {
		if candidate.preserved {
			continue
		}
		for _, dst := range candidate.entry.LinkedTo {
			if _, err := os.Lstat(dst); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return fmt.Errorf("preflight destination %s: %w", dst, err)
			}
			// A lock entry alone does not prove ownership: the user may have
			// replaced the link with real content since it was recorded.
			if prev, ok := owned[dst]; ok && linker.IsManagedLink(dst, prev) {
				continue
			}
			if linker.IsManagedLink(dst, candidate.entry.SkillPath) {
				continue
			}
			return fmt.Errorf("destination %s exists and is not managed by skimi; remove it manually to let skimi use this path", dst)
		}
	}

	newLinks := make(map[string]struct{})
	for _, winner := range winners {
		for _, link := range winner.entry.LinkedTo {
			newLinks[link] = struct{}{}
		}
	}
	var stale []string
	for _, skill := range old.Skills {
		for _, link := range skill.LinkedTo {
			if _, keep := newLinks[link]; !keep {
				stale = append(stale, link)
			}
		}
	}
	sort.Strings(stale)

	if opts.DryRun {
		for _, winner := range winners {
			if winner.preserved {
				continue
			}
			fmt.Println(ui.Yellow.Render("  Install skill " + winner.entry.Name))
			for i, dst := range winner.entry.LinkedTo {
				fmt.Println(ui.Dim.Render(linkLine("Would link", winner.entry.Name, winner.agents[i], dst)))
			}
		}
		for _, link := range stale {
			if _, err := os.Lstat(link); os.IsNotExist(err) {
				continue
			}
			if linker.IsManagedLink(link, owned[link]) {
				fmt.Println(ui.Dim.Render("  Would remove stale link " + shortPath(link)))
			} else {
				fmt.Println(ui.Dim.Render("  Would leave " + shortPath(link) + " (not managed by skimi)"))
			}
		}
		return nil
	}

	var created []string
	var replaced []replacedLink
	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_ = linker.RemoveLink(created[i])
		}
		for i := len(replaced) - 1; i >= 0; i-- {
			_ = os.MkdirAll(filepath.Dir(replaced[i].path), 0o755)
			_ = os.Rename(replaced[i].backup, replaced[i].path)
		}
	}

	for _, winner := range winners {
		if winner.preserved {
			continue
		}
		fmt.Println(ui.Yellow.Render("  Install skill " + winner.entry.Name))
		for i, dst := range winner.entry.LinkedTo {
			if _, err := os.Lstat(dst); err == nil {
				if opts.Verbose {
					fmt.Println(ui.Magenta.Render(linkLine("Replacing", winner.entry.Name, winner.agents[i], dst)))
				}
				backup := backupPath(dst)
				if err := os.Rename(dst, backup); err != nil {
					rollback()
					return fmt.Errorf("backup link %s: %w", dst, err)
				}
				replaced = append(replaced, replacedLink{path: dst, backup: backup})
			}
			if err := linker.CreateLink(winner.entry.SkillPath, dst); err != nil {
				rollback()
				return fmt.Errorf("create link for %s in agent %s: %w", winner.entry.Name, winner.agents[i], err)
			}
			created = append(created, dst)
			fmt.Println(linkLine("Linked", winner.entry.Name, winner.agents[i], dst))
		}
	}

	for _, link := range stale {
		if _, err := os.Lstat(link); os.IsNotExist(err) {
			continue
		}
		if !linker.IsManagedLink(link, owned[link]) {
			fmt.Fprintf(os.Stderr, "warning: leaving %s: not managed by skimi\n", link)
			continue
		}
		backup := backupPath(link)
		if err := os.Rename(link, backup); err != nil {
			rollback()
			return fmt.Errorf("backup stale link %s: %w", link, err)
		}
		replaced = append(replaced, replacedLink{path: link, backup: backup})
		removeEmptyParents(link)
	}

	entries := make([]types.InstalledSkill, len(winners))
	for i, winner := range winners {
		entries[i] = winner.entry
	}
	if err := lock.Save(opts.LockPath, &types.LockFile{Version: lock.CurrentVersion, Skills: entries}); err != nil {
		rollback()
		return fmt.Errorf("save lock file: %w", err)
	}
	for _, item := range replaced {
		if err := linker.RemoveLink(item.backup); err != nil {
			fmt.Fprintln(os.Stderr, "warning: remove transaction backup "+item.backup+": "+err.Error())
			continue
		}
		removeEmptyParents(item.backup)
	}
	return nil
}

func backupPath(path string) string {
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("%s.skimi-backup-%d", path, i)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func removeEmptyParents(path string) {
	parent := filepath.Dir(path)
	for {
		if filepath.Base(parent) == "skills" {
			return
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			return
		}
		if err := os.Remove(parent); err != nil {
			return
		}
		next := filepath.Dir(parent)
		if next == parent {
			return
		}
		parent = next
	}
}

// linkLine formats a link status line: "  <verb> <skill> -> [<agent>] <path>".
func linkLine(verb, skillName, agent, dstPath string) string {
	return fmt.Sprintf("  %s %s -> [%s] %s", verb, skillName, agent, shortPath(dstPath))
}

// ensureRepo clones the repo if dest does not exist, or pulls if it does.
func ensureRepo(repo, dest string) error {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return git.Clone(repo, dest)
	}
	return git.Pull(dest)
}

// shortPath replaces the home directory prefix with ~.
func shortPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
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

func selectSkills(all []types.DetectedSkill, selectors []types.SkillSelector) ([]types.DetectedSkill, error) {
	selected := make(map[string]types.DetectedSkill)
	for _, selector := range selectors {
		matched := false
		for _, skill := range all {
			if (selector.Name != "" && skill.Name == selector.Name) || (selector.Path != "" && skill.SourcePath == selector.Path) {
				selected[skill.SourcePath] = skill
				matched = true
			}
		}
		if !matched {
			if selector.Path != "" {
				return nil, fmt.Errorf("path %q did not match any skill", selector.Path)
			}
			return nil, fmt.Errorf("name %q did not match any skill", selector.Name)
		}
	}
	out := make([]types.DetectedSkill, 0, len(selected))
	for _, skill := range all {
		if _, ok := selected[skill.SourcePath]; ok {
			out = append(out, skill)
			delete(selected, skill.SourcePath)
		}
	}
	return out, nil
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
