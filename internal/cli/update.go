package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/config"
	"github.com/timonwong/skimi/internal/installer"
)

func newUpdateCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update all installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(globalConfigFile)
			if err != nil {
				return err
			}
			if len(cfg.Packages) == 0 {
				fmt.Println("No packages declared in config. Nothing to update.")
				return nil
			}

			opts := installer.Options{
				StoreDir: globalStoreDir,
				LockPath: globalLockFile,
				DryRun:   dryRun,
			}
			// installer.Run already performs git pull before installing.
			return installer.Run(cfg, opts)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}
