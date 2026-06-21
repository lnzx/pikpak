package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/lnzx/pikpak/cmd"
	"github.com/lnzx/pikpak/internal/config"
	"github.com/urfave/cli/v3"
)

var Version = "0.1.0"

func main() {
	root := &cli.Command{
		Name:            "pikpak",
		Usage:           "pikpak command-line client " + Version,
		Version:         Version,
		HideHelpCommand: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "account",
				Aliases: []string{"a"},
				Usage:   "account to use",
			},
		},
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			cfg, err := config.Load()
			if err != nil {
				return ctx, err
			}
			return config.WithContext(ctx, cfg), nil
		},
		Commands: []*cli.Command{
			cmd.AccountCmd,
			cmd.QuotaCmd,
			cmd.TaskCmd,
			cmd.FileCmd,
			cmd.TrashCmd,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := root.Run(ctx, os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
