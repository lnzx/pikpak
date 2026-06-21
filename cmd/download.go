package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lnzx/pikpak/internal/pikpak"
	"github.com/urfave/cli/v3"
)

var DownloadCmd = &cli.Command{
	Name:      "download",
	Aliases:   []string{"d"},
	Usage:     "download a file, or the direct child files of a folder, by id or remote path",
	ArgsUsage: "<file_id|path> [file_id|path...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "output path",
		},
		&cli.IntFlag{
			Name:    "parallel",
			Aliases: []string{"p"},
			Usage:   "parallel connections for ranged download (default 4, 1 disables)",
		},
		&cli.StringFlag{
			Name:    "chunk-min",
			Aliases: []string{"c"},
			Usage:   "minimum file size to use parallel mode, e.g. 32MB (default 32MB)",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "redownload even if local file size matches",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		if c.Args().Len() < 1 {
			return errors.New("download requires at least one <file_id|path> argument")
		}
		opts := pikpak.DownloadOptions{
			Parallel: c.Int("parallel"),
			Force:    c.Bool("force"),
		}
		if cm := c.String("chunk-min"); cm != "" {
			n, err := parseSize(cm)
			if err != nil {
				return fmt.Errorf("--chunk-min: %w", err)
			}
			opts.ChunkMin = n
		}
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		fmt.Printf("account: %s\n", acc.Alias)

		targets := c.Args().Slice()
		output := c.String("output")
		var errs []error
		for i, target := range targets {
			if len(targets) > 1 {
				fmt.Printf("\n[%d/%d] downloading: %s\n", i+1, len(targets), target)
			}
			if err := client.Download(ctx, target, output, opts); err != nil {
				fmt.Fprintf(c.ErrWriter, "error downloading %s: %v\n", target, err)
				errs = append(errs, fmt.Errorf("%s: %w", target, err))
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("failed to download %d of %d files", len(errs), len(targets))
		}
		return nil
	},
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, errors.New("empty size")
	}
	suffixes := []struct {
		s string
		m float64
	}{
		{"PB", 1 << 50},
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"P", 1 << 50},
		{"T", 1 << 40},
		{"G", 1 << 30},
		{"M", 1 << 20},
		{"K", 1 << 10},
		{"B", 1},
	}
	mul := 1.0
	num := s
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf.s) {
			mul = suf.m
			num = strings.TrimSpace(strings.TrimSuffix(s, suf.s))
			break
		}
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("size must be >= 0")
	}
	return int64(n * mul), nil
}
