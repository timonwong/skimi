package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/config"
	"github.com/timonwong/skimi/internal/detect"
	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/installer"
	"github.com/timonwong/skimi/internal/source"
	"github.com/timonwong/skimi/internal/types"
	"github.com/timonwong/skimi/internal/ui"
)

func newInstallCmd() *cobra.Command {
	var dryRun bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "install [source [skill...]]",
		Short: "Install skills from skills.yaml or interactively from a source",
		Long: `Install skills declared in skills.yaml.

When a source is provided (git repo or local path), skimi detects available
skills and lets you select which ones to install interactively.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := installer.Options{
				StoreDir: globalStoreDir,
				LockPath: globalLockFile,
				DryRun:   dryRun,
				Verbose:  verbose,
			}

			if len(args) == 0 {
				return runInstallFromConfig(opts)
			}
			return runInstallInteractive(args[0], args[1:], opts)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show additional installation detail")
	return cmd
}

// runInstallFromConfig reads skills.yaml and installs everything declared in it.
func runInstallFromConfig(opts installer.Options) error {
	cfg, err := config.Load(globalConfigFile)
	if err != nil {
		return err
	}
	if len(cfg.Packages) == 0 {
		fmt.Println("No packages declared in config. Nothing to install.")
		return nil
	}
	return installer.Run(cfg, opts)
}

// runInstallInteractive resolves the source, detects skills, presents a TUI
// multi-select, and installs the chosen skills. Unlike the config-driven
// path, it is additive: skills already installed stay untouched.
func runInstallInteractive(src string, preselect []string, opts installer.Options) error {
	opts.Additive = true
	// resolveSource below clones or pulls the repo, so the installer must not
	// sync it a second time. That single sync also fixes the policy conflict:
	// resolveSource warns and keeps the cached copy when a pull fails, whereas
	// the installer's sync is fatal, so offline runs used to abort right after
	// the user picked skills from the copy the warning told them to expect.
	opts.SkipSync = true

	// Resolve source to a local directory.
	sourceDir, isRemote, err := resolveSource(src, opts.StoreDir, opts.DryRun)
	if errors.Is(err, errDryRunNotCloned) {
		return nil
	}
	if err != nil {
		return err
	}

	detected, err := detect.Scan(sourceDir)
	if err != nil {
		return fmt.Errorf("detect skills: %w", err)
	}
	if len(detected) == 0 {
		fmt.Println("No skills found in", src)
		return nil
	}

	// If skills were given as arguments, validate and use them directly.
	selectedNames := preselect
	if len(selectedNames) == 0 {
		selectedNames, err = selectSkillsTUI(detected)
		if err != nil {
			return err
		}
	}
	if len(selectedNames) == 0 {
		fmt.Println("No skills selected.")
		return nil
	}

	// Build a minimal config for the chosen skills.
	// Use the original src to preserve any subdir information.
	pkg := types.SkillPackageConfig{
		Skills: make([]types.SkillSelector, len(selectedNames)),
	}
	for i, name := range selectedNames {
		pkg.Skills[i] = types.SkillSelector{Name: name}
	}
	if isRemote {
		pkg.Repo = src
	} else {
		pkg.LocalPath = src
	}

	cfg := &types.SkmConfig{
		Packages: []types.SkillPackageConfig{pkg},
	}
	if err := installer.Run(cfg, opts); err != nil {
		return err
	}
	if !opts.DryRun {
		fmt.Println(ui.Dim.Render("Note: this install is not recorded in skills.yaml; a config-driven `skimi install` removes skills not declared there."))
	}
	return nil
}

// selectSkillsTUI shows a charmbracelet/huh multi-select form and returns the
// chosen skill names.
func selectSkillsTUI(skills []types.DetectedSkill) ([]string, error) {
	options := make([]huh.Option[string], len(skills))
	for i, s := range skills {
		label := s.Name
		if s.Description != "" {
			label = fmt.Sprintf("%s — %s", s.Name, s.Description)
		}
		options[i] = huh.NewOption(label, s.Name)
	}

	var chosen []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select skills to install").
				Options(options...).
				Value(&chosen),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("TUI selection: %w", err)
	}
	return chosen, nil
}

// errDryRunNotCloned reports that a dry run stopped before the TUI because
// the source repo has no store copy yet: cloning is the very mutation a dry
// run promises not to make, and without a clone there is nothing real to
// select from.
var errDryRunNotCloned = errors.New("dry-run: source repo is not cloned")

// resolveSource returns the local directory for a source, cloning if needed.
// isRemote is true when the source was a git repo. A failed sync is a warning
// rather than an error whenever a usable clone survives it: the cached copy is
// good enough to browse and install from offline. Callers must set
// installer.Options.SkipSync afterwards, since this is the only sync the
// interactive commands perform. With dryRun set the store is never mutated:
// an existing copy gets a read-only fetch preview, a missing one returns
// errDryRunNotCloned.
func resolveSource(src, storeDir string, dryRun bool) (dir string, isRemote bool, err error) {
	parsed, err := source.Parse(src)
	if err != nil {
		return "", false, err
	}

	if parsed.Kind == source.SourceLocal {
		expanded, err := installer.ExpandPath(parsed.LocalPath)
		if err != nil {
			return "", false, err
		}
		return expanded, false, nil
	}

	// Remote repo: clone, update, or repair the store copy.
	dest := installer.RepoStorePath(storeDir, parsed.Repo)
	if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
		if dryRun {
			fmt.Println(ui.Yellow.Render("Would clone " + parsed.Repo))
			fmt.Println(ui.Dim.Render("  Nothing to preview until the repo is cloned; run without --dry-run first."))
			return "", true, errDryRunNotCloned
		}
		fmt.Println(ui.Blue.Render("Using " + parsed.Repo))
	} else {
		fmt.Println(ui.Blue.Render("Using existing " + parsed.Repo))
	}
	if dryRun {
		// Preview remote state read-only and browse the unmoved working tree,
		// which live skill symlinks point into.
		oldCommit, headErr := git.HeadCommit(dest)
		if headErr != nil {
			fmt.Fprintf(os.Stderr, "warning: read HEAD for %s: %v\n", parsed.Repo, headErr)
		} else {
			installer.DryRunFetch(dest, parsed.Repo, oldCommit)
		}
	} else if err := installer.EnsureRepo(storeDir, parsed.GetCloneURL(), dest); err != nil {
		// An offline sync is survivable as long as the cached copy is intact;
		// without one there is nothing to browse, so the error stands.
		if !git.IsRepoRoot(dest) {
			return "", false, err
		}
		fmt.Fprintln(os.Stderr, ui.Red.Render("  Warning: repo sync failed, using the cached copy: "+err.Error()))
	}

	// Apply subdir if specified
	if parsed.Subdir != "" {
		dest = filepath.Join(dest, parsed.Subdir)
	}

	return dest, true, nil
}
