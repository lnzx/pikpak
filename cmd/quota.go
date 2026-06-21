package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/lnzx/pikpak/internal/config"
	"github.com/lnzx/pikpak/internal/pikpak"
	"github.com/urfave/cli/v3"
)

var QuotaCmd = &cli.Command{
	Name:    "quota",
	Aliases: []string{"q"},
	Usage:   "query quota for account",
	Action: func(ctx context.Context, c *cli.Command) error {
		cfg := config.FromContext(ctx)
		sessionDir, err := config.SessionsDir()
		if err != nil {
			return err
		}
		var targets []config.Account
		if alias := c.String("account"); alias != "" {
			acc, err := cfg.FindAccount(alias)
			if err != nil {
				return err
			}
			targets = []config.Account{acc}
		} else {
			targets = cfg.AllAccounts()
		}
		if len(targets) == 0 {
			fmt.Println("no accounts configured")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ACCOUNT\tCLOUD_DOWNLOAD(REMAINING/TOTAL)\tSTORAGE")
		for _, acc := range targets {
			client := pikpak.New(acc, sessionDir)
			if err := client.Login(ctx); err != nil {
				fmt.Fprintf(w, "%s\tERROR\t%s\n", acc.Alias, err)
				continue
			}
			q, err := client.Quota(ctx)
			if err != nil {
				fmt.Fprintf(w, "%s\tERROR\t%s\n", acc.Alias, err)
				continue
			}
			fmt.Fprintf(w, "%s\t%d/%d\t%s/%s\n",
				acc.Alias,
				q.Quotas.CloudDownload.Limit-q.Quotas.CloudDownload.Usage,
				q.Quotas.CloudDownload.Limit,
				pikpak.ByteSize(q.Quota.Usage),
				pikpak.ByteSize(q.Quota.Limit),
			)
		}
		return w.Flush()
	},
}
