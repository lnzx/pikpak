package session

import (
	"os"
	"testing"
	"time"
)

func TestSaveLoadAndExists(t *testing.T) {
	dir := t.TempDir()
	account := "user@example.com"
	data := &Data{
		AccessToken:  "access",
		RefreshToken: "refresh",
		CaptchaToken: "captcha",
		UserID:       "user-id",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}

	if Exists(dir, account) {
		t.Fatal("session should not exist before save")
	}
	if err := Save(dir, account, data); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir, account) {
		t.Fatal("session should exist after save")
	}

	loaded, err := Load(dir, account)
	if err != nil {
		t.Fatal(err)
	}
	if *loaded != *data {
		t.Fatalf("loaded session = %#v", loaded)
	}

	if _, err := os.Stat(FilePath(dir, account)); err != nil {
		t.Fatal(err)
	}
}

func TestExpired(t *testing.T) {
	now := time.Now()
	if !(*Data)(nil).Expired(now) {
		t.Fatal("nil data should be expired")
	}
	if !(&Data{}).Expired(now) {
		t.Fatal("empty token should be expired")
	}
	if !(&Data{AccessToken: "token", ExpiresAt: now.Add(-time.Second).Unix()}).Expired(now) {
		t.Fatal("past expiry should be expired")
	}
	if (&Data{AccessToken: "token", ExpiresAt: now.Add(time.Hour).Unix()}).Expired(now) {
		t.Fatal("future expiry should not be expired")
	}
}
