package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/config"
	"github.com/timonwong/skimi/internal/git"
	"github.com/timonwong/skimi/internal/installer"
	"github.com/timonwong/skimi/internal/lock"
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

			// Build a commit map from the lock file.
			lockedCommits := make(map[string]string)
			for _, s := range lf.Skills {
				if s.Repo != "" && s.Commit != "" {
					lockedCommits[s.Repo] = s.Commit
				}
			}

			anyUpdate := false

			for _, pkg := range cfg.Packages {
				if pkg.Repo == "" {
					continue
				}
				dest := installer.RepoStorePath(globalStoreDir, pkg.Repo)
				if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
					fmt.Printf("%-40s  not cloned\n", pkg.Repo)
					continue
				}

				fmt.Printf("Fetching %s ...\n", pkg.Repo)
				if err := git.Fetch(dest); err != nil {
					fmt.Fprintf(os.Stderr, "warning: fetch %s: %v\n", pkg.Repo, err)
					continue
				}

				localCommit := lockedCommits[pkg.Repo]
				// After git fetch, FETCH_HEAD contains the fetched commit.
				remoteCommit, err := git.RevParse(dest, "FETCH_HEAD")
				if err != nil {
					// Fall back to HEAD.
					remoteCommit, err = git.HeadCommit(dest)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: get remote HEAD for %s: %v\n", pkg.Repo, err)
						continue
					}
				}

				if localCommit == remoteCommit {
					fmt.Printf("  %s is up to date.\n", pkg.Repo)
					continue
				}

				anyUpdate = true
				fmt.Printf("  %s has updates:\n", pkg.Repo)
				if localCommit != "" {
					log, _ := git.Log(dest, localCommit, remoteCommit)
					if log != "" {
						for _, line := range splitLines(log) {
							fmt.Printf("    %s\n", line)
						}
					}
				}
			}

			if !anyUpdate {
				fmt.Println("\nAll skills are up to date.")
			} else {
				fmt.Println("\nRun `skimi update` to apply updates.")
			}
			return nil
		},
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
