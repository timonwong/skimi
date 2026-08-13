package cli

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/linker"
	"github.com/timonwong/skimi/internal/lock"
	"github.com/timonwong/skimi/internal/types"
)

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <skill-name...>",
		Short: "Remove one or more installed skills and their links",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lf, err := lock.Load(globalLockFile)
			if err != nil {
				return err
			}

			remaining, removed := removeSkills(lf, args)

			if removed == 0 {
				fmt.Println("No matching skills found in lock file.")
				return nil
			}

			newLF := &types.LockFile{Version: lf.Version, Skills: remaining}
			if err := lock.Save(globalLockFile, newLF); err != nil {
				return fmt.Errorf("save lock file: %w", err)
			}
			fmt.Printf("Removed %d skill(s).\n", removed)
			return nil
		},
	}
}

// removeSkills deletes the links of the named skills after verifying skimi
// still owns them, and returns the remaining lock entries.
func removeSkills(lf *types.LockFile, names []string) (remaining []types.InstalledSkill, removed int) {
	for _, s := range lf.Skills {
		if !slices.Contains(names, s.Name) {
			remaining = append(remaining, s)
			continue
		}

		fmt.Printf("Removing skill %q ...\n", s.Name)
		for _, link := range s.LinkedTo {
			if _, err := os.Lstat(link); os.IsNotExist(err) {
				continue
			}
			if !linker.IsManagedLink(link, s.SkillPath) {
				fmt.Fprintf(os.Stderr, "  warning: leaving %s: not managed by skimi\n", link)
				continue
			}
			if err := linker.RemoveLink(link); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: remove link %s: %v\n", link, err)
			} else {
				fmt.Printf("  removed link: %s\n", link)
			}
		}
		removed++
	}
	return remaining, removed
}
