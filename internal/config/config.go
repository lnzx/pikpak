package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

const AppName = "pikpak"

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", AppName), nil
}

func configFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func SessionsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

type Account struct {
	Alias    string `toml:"-"`
	Username string
	Password string
}

type Config struct {
	Accounts map[string]Account `toml:"accounts"`
}

func Load() (*Config, error) {
	file, err := configFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", file, err)
	}
	var c Config
	if err = toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", file, err)
	}
	return &c, nil
}

func (c *Config) FindAccount(alias string) (Account, error) {
	if acc, ok := c.Accounts[alias]; ok {
		acc.Alias = alias
		return acc, nil
	}
	return Account{}, fmt.Errorf("account %q not found", alias)
}

func (c *Config) Select(alias string) (Account, error) {
	if alias != "" {
		return c.FindAccount(alias)
	}
	list := c.AllAccounts()
	if len(list) == 0 {
		return Account{}, fmt.Errorf("no accounts configured")
	}
	return list[0], nil
}

func (c *Config) AllAccounts() []Account {
	list := make([]Account, 0, len(c.Accounts))
	for alias, acc := range c.Accounts {
		acc.Alias = alias
		list = append(list, acc)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Alias < list[j].Alias })
	return list
}

type ctxKey struct{}

func WithContext(ctx context.Context, c *Config) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

func FromContext(ctx context.Context) *Config {
	c, _ := ctx.Value(ctxKey{}).(*Config)
	return c
}
