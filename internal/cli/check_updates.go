package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/config"
	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/installer"
	"github.com/timonwong/skimi/internal/lock"
	"github.com/timonwong/skimi/internal/source"
	"github.com/timonwong/skimi/internal/types"
)

func newCheckUpdatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-updates",
		Short: "Check for available skill updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(globalConfigFile)
			if err != nil {
				return err
			}
			lf, err := lock.Load(globalLockFile)
			if err != nil {
				return err
			}

			// Build a commit map from the lock file, keyed by the normalized
			// repo identifier (the same key installer writes to the lock).
			lockedCommits := make(map[string]string)
			for _, s := range lf.Skills {
				if s.Repo != "" && s.Commit != "" {
					lockedCommits[s.Repo] = s.Commit
				}
			}

			repos, err := normalizeCheckUpdateRepos(cfg.Packages)
			if err != nil {
				return err
			}

			anyUpdate := false

			for _, repo := range repos {
				dest := installer.RepoStorePath(globalStoreDir, repo)
				if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
					fmt.Printf("%-40s  not cloned\n", repo)
					continue
				}

				fmt.Printf("Fetching %s ...\n", repo)
				if err := git.Fetch(dest); err != nil {
					fmt.Fprintf(os.Stderr, "warning: fetch %s: %v\n", repo, err)
					continue
				}

				localCommit := lockedCommits[repo]
				// After git fetch, FETCH_HEAD contains the fetched commit.
				remoteCommit, err := git.RevParse(dest, "FETCH_HEAD")
				if err != nil {
					// Fall back to HEAD.
					remoteCommit, err = git.HeadCommit(dest)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: get remote HEAD for %s: %v\n", repo, err)
						continue
					}
				}

				if localCommit == remoteCommit {
					fmt.Printf("  %s is up to date.\n", repo)
					continue
				}

				anyUpdate = true
				fmt.Printf("  %s has updates:\n", repo)
				if localCommit != "" {
					log, _ := git.Log(dest, localCommit, remoteCommit)
					if log != "" {
						for _, line := range strings.Split(log, "\n") {
							fmt.Printf("    %s\n", line)
						}
					}
				}
			}

			if !anyUpdate {
				fmt.Println("\nAll skills are up to date.")
			} else {
				fmt.Println("\n" + updateHint())
			}
			return nil
		},
	}
}

// normalizeCheckUpdateRepos parses each package's repo string through
// source.Parse and returns the deduplicated, normalized remote repo
// identifiers in first-seen order. Packages without a repo, and packages
// resolving to a local source, are skipped. This mirrors how
// installer.preparePackage and filterUpdateReposByConfig normalize repo
// strings, so check-updates computes the same store path and lock key that
// install/update use for shorthand (owner/repo), URL (https/https+.git/
// git@), and subdir config forms. Packages that share a repo (e.g. via
// different subdirs) collapse to a single entry so the repo is fetched and
// reported once.
func normalizeCheckUpdateRepos(pkgs []types.SkillPackageConfig) ([]string, error) {
	seen := make(map[string]struct{}, len(pkgs))
	var repos []string
	for _, pkg := range pkgs {
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
		if _, ok := seen[parsed.Repo]; ok {
			continue
		}
		seen[parsed.Repo] = struct{}{}
		repos = append(repos, parsed.Repo)
	}
	return repos, nil
}

func updateHint() string {
	return "Run `skimi update --all` to apply updates."
}
