package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

var TrashCmd = &cli.Command{
	Name:  "trash",
	Usage: "manage trash",
	Commands: []*cli.Command{
		emptyCmd,
	},
}

var emptyCmd = &cli.Command{
	Name:  "empty",
	Usage: "empty trash",
	Action: func(ctx context.Context, c *cli.Command) error {
		p, err := poolFromContext(ctx, c)
		if err != nil {
			return err
		}

		// Single-account mode.
		if p == nil {
			client, acc, err := clientFromContext(ctx, c)
			if err != nil {
				return err
			}
			if err := client.EmptyTrash(ctx); err != nil {
				return err
			}
			fmt.Printf("account: %s empty Trash OK\n", acc.Alias)
			return nil
		}

		// Multi-account mode.
		clients, accounts, err := p.ClientsForAll(ctx)
		if err != nil {
			return err
		}
		for i, client := range clients {
			if err := client.EmptyTrash(ctx); err != nil {
				fmt.Printf("account: %s error=%v\n", accounts[i].Alias, err)
				continue
			}
			fmt.Printf("account: %s empty Trash OK\n", accounts[i].Alias)
		}
		return nil
	},
}
