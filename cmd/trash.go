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
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		err = client.EmptyTrash(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("account: %s empty Trash OK\n", acc.Alias)
		return nil
	},
}
