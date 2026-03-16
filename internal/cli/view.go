package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/detect"
	"github.com/timonwong/skimi/internal/git"
)

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <source>",
		Short: "Preview skills available in a source without installing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			sourceDir, _, err := resolveSource(source, globalStoreDir)
			if err != nil {
				return err
			}

			// Show HEAD commit if it is a repo.
			if commit, err := git.HeadCommit(sourceDir); err == nil {
				short := commit
				if len(short) > 8 {
					short = short[:8]
				}
				fmt.Fprintf(os.Stdout, "Source: %s @ %s\n\n", source, short)
			} else {
				fmt.Fprintf(os.Stdout, "Source: %s\n\n", source)
			}

			skills, err := detect.Scan(sourceDir)
			if err != nil {
				return fmt.Errorf("detect skills: %w", err)
			}
			if len(skills) == 0 {
				fmt.Println("No skills found.")
				return nil
			}

			fmt.Printf("Found %d skill(s):\n", len(skills))
			for _, s := range skills {
				if s.Description != "" {
					fmt.Printf("  • %s — %s\n", s.Name, s.Description)
				} else {
					fmt.Printf("  • %s\n", s.Name)
				}
			}
			return nil
		},
	}
}
