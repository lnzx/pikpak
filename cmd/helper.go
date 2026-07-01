package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lnzx/pikpak/internal/config"
	"github.com/lnzx/pikpak/internal/pikpak"
	"github.com/lnzx/pikpak/internal/pool"
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

// resolveFileIDAccount finds which account owns the given file_id. Resolution
// order: (1) explicit -a flag, (2) exact match in local task state, (3) longest
// common-prefix match against known file_ids (threshold 20 chars — same-account
// file_ids share ≥22 of 26 chars), (4) API probe across all accounts.
func resolveFileIDAccount(ctx context.Context, c *cli.Command, fileID string) (*pikpak.Client, config.Account, error) {
	// If user explicitly specified an account, use it.
	if c.String("account") != "" {
		return clientFromContext(ctx, c)
	}

	cfg := config.FromContext(ctx)
	accounts := cfg.AllAccounts()
	if len(accounts) == 0 {
		return nil, config.Account{}, fmt.Errorf("no accounts configured")
	}

	sessionDir, err := config.SessionsDir()
	if err != nil {
		return nil, config.Account{}, err
	}

	// Try local task state first (zero API calls).
	if state, err := pool.LoadState(); err == nil {
		// Step 2: exact file_id match.
		if owner := state.FindFileOwner(fileID); owner != "" {
			if client, acc, err := clientForAccount(owner, accounts, sessionDir, ctx); err == nil {
				return client, acc, nil
			}
		}
		// Step 3: prefix match against known file_ids.
		if owner := state.FindFileOwnerByPrefix(fileID); owner != "" {
			if client, acc, err := clientForAccount(owner, accounts, sessionDir, ctx); err == nil {
				return client, acc, nil
			}
		}
	}

	// Step 4: fall back to API probe across all accounts.
	var lastErr error
	for _, acc := range accounts {
		client := pikpak.New(acc, sessionDir)
		if err := client.Login(ctx); err != nil {
			lastErr = err
			continue
		}
		if _, err := client.File(ctx, fileID); err == nil {
			return client, acc, nil
		} else {
			lastErr = err
		}
	}
	return nil, config.Account{}, fmt.Errorf("file_id %s not found in any account (last error: %w)", fileID, lastErr)
}

// clientForAccount builds and logs in a pikpak.Client for the given alias.
func clientForAccount(alias string, accounts []config.Account, sessionDir string, ctx context.Context) (*pikpak.Client, config.Account, error) {
	for _, acc := range accounts {
		if acc.Alias == alias {
			client := pikpak.New(acc, sessionDir)
			if err := client.Login(ctx); err != nil {
				return nil, config.Account{}, err
			}
			return client, acc, nil
		}
	}
	return nil, config.Account{}, fmt.Errorf("account %q not found", alias)
}

// poolFromContext returns an AccountPool when the user did NOT specify -a.
// When -a is given (alias != ""), it returns nil — callers should use
// clientFromContext instead for single-account mode.
func poolFromContext(ctx context.Context, c *cli.Command) (*pool.AccountPool, error) {
	if c.String("account") != "" {
		return nil, nil // caller checks: nil pool means single-account mode
	}
	cfg := config.FromContext(ctx)
	accounts := cfg.AllAccounts()
	if len(accounts) == 0 {
		return nil, nil
	}
	sessionDir, err := config.SessionsDir()
	if err != nil {
		return nil, err
	}
	return pool.New(accounts, sessionDir), nil
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
