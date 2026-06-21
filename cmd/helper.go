package cmd

import (
	"bufio"
	"context"
	"os"
	"strings"

	"github.com/lnzx/pikpak/internal/config"
	"github.com/lnzx/pikpak/internal/pikpak"
	"github.com/urfave/cli/v3"
)

var forceFlag = &cli.BoolFlag{
	Name:    "force",
	Aliases: []string{"f"},
	Usage:   "permanently delete files instead of moving to trash",
}

var deleteFilesFlag = &cli.BoolFlag{
	Name:    "delete-files",
	Aliases: []string{"d"},
	Usage:   "also delete downloaded files",
}

func clientFromContext(ctx context.Context, c *cli.Command) (*pikpak.Client, config.Account, error) {
	cfg := config.FromContext(ctx)
	acc, err := cfg.Select(c.String("account"))
	if err != nil {
		return nil, config.Account{}, err
	}
	sessionDir, err := config.SessionsDir()
	if err != nil {
		return nil, config.Account{}, err
	}
	client := pikpak.New(acc, sessionDir)
	if err = client.Login(ctx); err != nil {
		return nil, config.Account{}, err
	}
	return client, acc, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
