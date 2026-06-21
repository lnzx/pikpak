package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/lnzx/pikpak/internal/config"
	"github.com/lnzx/pikpak/internal/session"
	"github.com/urfave/cli/v3"
)

var AccountCmd = &cli.Command{
	Name:    "accounts",
	Aliases: []string{"acc"},
	Usage:   "list configured accounts",
	Action: func(ctx context.Context, c *cli.Command) error {
		cfg := config.FromContext(ctx)
		accounts := cfg.AllAccounts()
		if len(accounts) == 0 {
			fmt.Println("no accounts configured")
			return nil
		}
		sessionDir, err := config.SessionsDir()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ALIAS\tUSERNAME\tSESSION")
		for _, acc := range accounts {
			state := "-"
			if session.Exists(sessionDir, acc.Username) {
				state = "cached"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", acc.Alias, acc.Username, state)
		}
		return w.Flush()
	},
}
