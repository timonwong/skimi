package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/config"
	"github.com/timonwong/skimi/internal/installer"
	"github.com/timonwong/skimi/internal/lock"
	"github.com/timonwong/skimi/internal/source"
	"github.com/timonwong/skimi/internal/types"
	"github.com/timonwong/skimi/internal/ui"
)

func newUpdateCmd() *cobra.Command {
	var dryRun bool
	var verbose bool
	var updateAll bool

	cmd := &cobra.Command{
		Use:   "update [skill-name...]",
		Short: "Update installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !updateAll {
				return fmt.Errorf("provide skill name(s) or use --all")
			}
			if len(args) > 0 && updateAll {
				return fmt.Errorf("provide skill name(s) or use --all")
			}

			cfg, err := config.Load(globalConfigFile)
			if err != nil {
				return err
			}
			if len(cfg.Packages) == 0 {
				fmt.Println("No packages declared in config. Nothing to update.")
				return nil
			}
			lf, err := lock.Load(globalLockFile)
			if err != nil {
				return err
			}

			selection, err := selectUpdateRepos(lf, args, updateAll)
			if err != nil {
				return err
			}
			for _, name := range selection.LocalSkills {
				fmt.Println(ui.Dim.Render(fmt.Sprintf("Skill %q is from a local path, skipping update.", name)))
			}
			repos, skippedRepos, err := filterUpdateReposByConfig(cfg, selection.Repos)
			if err != nil {
				return err
			}
			for _, repo := range skippedRepos {
				fmt.Println(ui.Dim.Render(fmt.Sprintf("Repo %q is not declared in config, skipping update.", repo)))
			}
			if len(repos) == 0 {
				fmt.Println("Nothing to update.")
				return nil
			}

			opts := installer.Options{
				StoreDir: globalStoreDir,
				LockPath: globalLockFile,
				DryRun:   dryRun,
				Verbose:  verbose,
			}
			return installer.UpdateRepos(cfg, repos, opts)
		},
	}

	cmd.Flags().BoolVar(&updateAll, "all", false, "update all installed remote skills")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show additional installation detail")
	return cmd
}

type updateRepoSelection struct {
	Repos       []string
	LocalSkills []string
}

func selectUpdateRepos(lf *types.LockFile, skillNames []string, updateAll bool) (updateRepoSelection, error) {
	var selection updateRepoSelection
	seenRepos := make(map[string]struct{})

	addRepo := func(repo string) {
		if repo == "" {
			return
		}
		if _, ok := seenRepos[repo]; ok {
			return
		}
		seenRepos[repo] = struct{}{}
		selection.Repos = append(selection.Repos, repo)
	}

	if updateAll {
		for _, skill := range lf.Skills {
			addRepo(skill.Repo)
		}
		return selection, nil
	}

	for _, name := range skillNames {
		skill := lock.FindByName(lf, name)
		if skill == nil {
			return updateRepoSelection{}, fmt.Errorf("skill '%s' is not installed", name)
		}
		if skill.Repo == "" {
			selection.LocalSkills = append(selection.LocalSkills, name)
			continue
		}
		addRepo(skill.Repo)
	}

	return selection, nil
}

func filterUpdateReposByConfig(cfg *types.SkmConfig, repos []string) ([]string, []string, error) {
	declared := make(map[string]struct{})
	for _, pkg := range cfg.Packages {
		if pkg.Repo == "" {
			continue
		}
		parsed, err := source.Parse(pkg.Repo)
		if err != nil {
			return nil, nil, fmt.Errorf("parse repo %q: %w", pkg.Repo, err)
		}
		if parsed.Kind == source.SourceRemote {
			declared[parsed.Repo] = struct{}{}
		}
	}

	var filtered []string
	var skipped []string
	for _, repo := range repos {
		if _, ok := declared[repo]; ok {
			filtered = append(filtered, repo)
			continue
		}
		skipped = append(skipped, repo)
	}
	return filtered, skipped, nil
}
