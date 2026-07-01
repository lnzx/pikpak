package pool

import (
	"context"
	"fmt"
	"sort"

	"github.com/lnzx/pikpak/internal/config"
	"github.com/lnzx/pikpak/internal/pikpak"
)

// AccountPool manages multi-account scheduling.
type AccountPool struct {
	accounts   []config.Account
	sessionDir string
}

// New creates an AccountPool for the given accounts.
func New(accounts []config.Account, sessionDir string) *AccountPool {
	return &AccountPool{accounts: accounts, sessionDir: sessionDir}
}

// Accounts returns the underlying account list.
func (p *AccountPool) Accounts() []config.Account {
	return p.accounts
}

// ClientFor creates and logs in a Client for the given account.
func (p *AccountPool) ClientFor(ctx context.Context, acc config.Account) (*pikpak.Client, error) {
	client := pikpak.New(acc, p.sessionDir)
	if err := client.Login(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

// SelectForAdd picks the best account for a new offline task.
// It reads the cached quota from task state and returns the account
// with the most remaining cloud_download slots.
// Falls back to the first uncached account when all cached accounts
// have no remaining slots. Callers should update the cache with
// QuotaSnapshot after successful task submission.
func (p *AccountPool) SelectForAdd(ctx context.Context) (config.Account, error) {
	if len(p.accounts) == 0 {
		return config.Account{}, fmt.Errorf("no accounts configured")
	}

	state, err := LoadState()
	if err != nil {
		return config.Account{}, err
	}

	// Build candidates with remaining quota from cache.
	type candidate struct {
		acc       config.Account
		remaining int64 // limit - usage; -1 means no cache
	}
	var candidates []candidate
	for _, acc := range p.accounts {
		rem := int64(-1)
		if as := state[acc.Alias]; as != nil && as.QuotaCache != nil {
			q := as.QuotaCache
			rem = q.CloudDownloadLimit - q.CloudDownloadUsage
		}
		candidates = append(candidates, candidate{acc: acc, remaining: rem})
	}

	// Sort: accounts with cache first, by remaining desc; uncached last (alphabetical).
	sort.Slice(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		if ci.remaining >= 0 && cj.remaining >= 0 {
			return ci.remaining > cj.remaining
		}
		if ci.remaining >= 0 {
			return true
		}
		if cj.remaining >= 0 {
			return false
		}
		return ci.acc.Alias < cj.acc.Alias
	})

	// If the best cached account has no remaining slots, fall back to an
	// uncached account (it may still have quota — we haven't queried it yet).
	best := candidates[0]
	if best.remaining <= 0 {
		for _, c := range candidates {
			if c.remaining < 0 {
				return c.acc, nil
			}
		}
		return config.Account{}, fmt.Errorf("all accounts have exhausted cloud download quota")
	}

	return best.acc, nil
}

// ClientsForAll returns logged-in clients for every configured account.
func (p *AccountPool) ClientsForAll(ctx context.Context) ([]*pikpak.Client, []config.Account, error) {
	var clients []*pikpak.Client
	var accounts []config.Account
	for _, acc := range p.accounts {
		client, err := p.ClientFor(ctx, acc)
		if err != nil {
			return nil, nil, fmt.Errorf("login %s: %w", acc.Alias, err)
		}
		clients = append(clients, client)
		accounts = append(accounts, acc)
	}
	return clients, accounts, nil
}

// ClientsForAccounts returns logged-in clients for the given aliases.
func (p *AccountPool) ClientsForAccounts(ctx context.Context, aliases []string) ([]*pikpak.Client, []config.Account, error) {
	byAlias := make(map[string]config.Account, len(p.accounts))
	for _, acc := range p.accounts {
		byAlias[acc.Alias] = acc
	}
	var clients []*pikpak.Client
	var accounts []config.Account
	for _, alias := range aliases {
		acc, ok := byAlias[alias]
		if !ok {
			continue
		}
		client, err := p.ClientFor(ctx, acc)
		if err != nil {
			return nil, nil, fmt.Errorf("login %s: %w", alias, err)
		}
		clients = append(clients, client)
		accounts = append(accounts, acc)
	}
	return clients, accounts, nil
}

// ClientForAlias returns a logged-in client for a specific alias.
func (p *AccountPool) ClientForAlias(ctx context.Context, alias string) (*pikpak.Client, config.Account, error) {
	for _, acc := range p.accounts {
		if acc.Alias == alias {
			client, err := p.ClientFor(ctx, acc)
			return client, acc, err
		}
	}
	return nil, config.Account{}, fmt.Errorf("account %q not found", alias)
}
