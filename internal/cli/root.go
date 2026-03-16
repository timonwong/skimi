package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/config"
)

var (
	globalConfigFile string
	globalLockFile   string
	globalStoreDir   string
)

var rootCmd = &cobra.Command{
	Use:   "skimi",
	Short: "A skill manager for AI agents",
	Long: `skimi manages AI agent skills across multiple agent platforms.

It reads a declarative configuration file (skills.yaml) and installs skills
from git repositories or local paths into agent-specific skill directories.

Inspired by reorx/skm (https://github.com/reorx/skm).`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not resolve default paths:", err)
	}

	rootCmd.PersistentFlags().StringVar(&globalConfigFile, "config", paths.ConfigFile, "config file path")
	rootCmd.PersistentFlags().StringVar(&globalLockFile, "lock", paths.LockFile, "lock file path")
	rootCmd.PersistentFlags().StringVar(&globalStoreDir, "store", paths.StoreDir, "skill store directory")

	rootCmd.AddCommand(
		newInstallCmd(),
		newListCmd(),
		newViewCmd(),
		newCheckUpdatesCmd(),
		newUpdateCmd(),
		newRemoveCmd(),
	)
}
