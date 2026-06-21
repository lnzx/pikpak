package session

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ExpirySkew = 5 * time.Minute

type Data struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	CaptchaToken string `json:"captcha_token"`
	UserID       string `json:"user_id"`
	ExpiresAt    int64  `json:"expires_at"`
}

func Load(dir, accountKey string) (*Data, error) {
	bs, err := os.ReadFile(FilePath(dir, accountKey))
	if err != nil {
		return nil, err
	}
	var data Data
	if err := json.Unmarshal(bs, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func Save(dir, accountKey string, data *Data) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	bs, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	path := FilePath(dir, accountKey)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bs, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Exists(dir, accountKey string) bool {
	_, err := os.Stat(FilePath(dir, accountKey))
	return err == nil
}

func FilePath(dir, accountKey string) string {
	sum := md5.Sum([]byte(accountKey))
	return filepath.Join(dir, fmt.Sprintf("session_%s.json", hex.EncodeToString(sum[:])))
}

func (d *Data) Expired(now time.Time) bool {
	if d == nil || d.AccessToken == "" {
		return true
	}
	return now.Unix() >= d.ExpiresAt
}
