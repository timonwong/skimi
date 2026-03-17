package cli

import (
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
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show override notices for existing links")
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
// multi-select, and installs the chosen skills.
func runInstallInteractive(src string, preselect []string, opts installer.Options) error {
	// Resolve source to a local directory.
	sourceDir, isRemote, err := resolveSource(src, opts.StoreDir)
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
		Skills: selectedNames,
	}
	if isRemote {
		pkg.Repo = src
	} else {
		pkg.LocalPath = src
	}

	cfg := &types.SkmConfig{
		Packages: []types.SkillPackageConfig{pkg},
	}
	return installer.Run(cfg, opts)
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

// resolveSource returns the local directory for a source, cloning if needed.
// isRemote is true when the source was a git repo.
func resolveSource(src, storeDir string) (dir string, isRemote bool, err error) {
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

	// Remote repo: clone/update
	dest := installer.RepoStorePath(storeDir, parsed.Repo)
	if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
		fmt.Println(ui.Blue.Render("Using " + parsed.Repo))
		if err := git.Clone(parsed.GetCloneURL(), dest); err != nil {
			return "", false, err
		}
	} else {
		fmt.Println(ui.Blue.Render("Using existing " + parsed.Repo))
		if err := git.Pull(dest); err != nil {
			fmt.Fprintln(os.Stderr, ui.Red.Render("  Warning: git pull failed: "+err.Error()))
		}
	}

	// Apply subdir if specified
	if parsed.Subdir != "" {
		dest = filepath.Join(dest, parsed.Subdir)
	}

	return dest, true, nil
}
