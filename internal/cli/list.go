package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"os"

	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/lock"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			lf, err := lock.Load(globalLockFile)
			if err != nil {
				return err
			}
			if len(lf.Skills) == 0 {
				fmt.Println("No skills installed.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSOURCE\tCOMMIT\tLINKS")
			fmt.Fprintln(w, "----\t------\t------\t-----")
			for _, s := range lf.Skills {
				source := s.Repo
				if source == "" {
					source = s.LocalPath
				}
				commit := s.Commit
				if len(commit) > 8 {
					commit = commit[:8]
				}
				links := strings.Join(s.LinkedTo, "\n\t\t\t")
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, source, commit, links)
			}
			return w.Flush()
		},
	}
}
