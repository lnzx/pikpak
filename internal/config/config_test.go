package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".config", "pikpak")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestLoad(t *testing.T) {
	writeConfig(t, `
[accounts]
main   = { username = "alice@example.com", password = "pass1" }
backup = { username = "bob@example.com",   password = "pass2" }
`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(cfg.Accounts), 2; got != want {
		t.Fatalf("got %d accounts, want %d", got, want)
	}
	if got := cfg.Accounts["main"]; got.Username != "alice@example.com" || got.Password != "pass1" {
		t.Errorf("accounts[main] = %+v", got)
	}
	if got := cfg.Accounts["backup"]; got.Username != "bob@example.com" || got.Password != "pass2" {
		t.Errorf("accounts[backup] = %+v", got)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	writeConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Accounts) != 0 {
		t.Errorf("got %d accounts, want 0", len(cfg.Accounts))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("error %q should be wrapped with read context", err)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	writeConfig(t, "this is not = valid [[ toml")
	_, err := Load()
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("error %q should be wrapped with parse context", err)
	}
}

func TestFindAccount(t *testing.T) {
	cfg := &Config{
		Accounts: map[string]Account{
			"main": {Username: "alice", Password: "pass1"},
		},
	}
	acc, err := cfg.FindAccount("main")
	if err != nil {
		t.Fatalf("FindAccount: %v", err)
	}
	if acc.Alias != "main" || acc.Username != "alice" || acc.Password != "pass1" {
		t.Errorf("got %+v", acc)
	}
}

func TestFindAccount_NotFound(t *testing.T) {
	cfg := &Config{Accounts: map[string]Account{
		"main": {Username: "alice", Password: "pass1"},
	}}
	_, err := cfg.FindAccount("missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `"missing"`) {
		t.Errorf("error %q should mention alias", err)
	}
}

func TestFindAccount_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	_, err := cfg.FindAccount("anything")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAllAccounts(t *testing.T) {
	cfg := &Config{Accounts: map[string]Account{
		"main":    {Username: "alice", Password: "p1"},
		"backup":  {Username: "bob", Password: "p2"},
		"archive": {Username: "carol", Password: "p3"},
	}}
	got := cfg.AllAccounts()
	wantAliases := []string{"archive", "backup", "main"}
	if len(got) != len(wantAliases) {
		t.Fatalf("got %d accounts, want %d", len(got), len(wantAliases))
	}
	for i, want := range wantAliases {
		if got[i].Alias != want {
			t.Errorf("accounts[%d].Alias = %q, want %q (slice should be alias-sorted)", i, got[i].Alias, want)
		}
	}
	if got[2].Username != "alice" || got[2].Password != "p1" {
		t.Errorf("accounts[2] = %+v, want main/alice/p1", got[2])
	}
}

func TestAllAccounts_Empty(t *testing.T) {
	cfg := &Config{}
	got := cfg.AllAccounts()
	if got == nil {
		t.Fatal("AllAccounts should return empty slice, not nil")
	}
	if len(got) != 0 {
		t.Errorf("got %d accounts, want 0", len(got))
	}
}
