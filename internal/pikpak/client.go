package pikpak

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lnzx/pikpak/internal/config"
	"github.com/lnzx/pikpak/internal/session"
)

const (
	apiDrive         = "https://api-drive.mypikpak.com"
	apiUser          = "https://user.mypikpak.com"
	clientID         = "YNxT9w7GMdWvEOKa"
	clientSecret     = "dbw2OtmVEeuUvIptb1Coyg"
	clientVersion    = "1.21.0"
	packageName      = "com.pikcloud.pikpak"
	userAgent        = "ANDROID-com.pikcloud.pikpak/1.21.0"
	fileKindFile     = "drive#file"
	FileKindFolder   = "drive#folder"
	defaultListLimit = "500"
	DefaultFolder    = "My Pack"
)

var defaultAPIClient = &http.Client{
	Timeout: 120 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConnsPerHost:   8,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

var captchaSalts = []string{
	"",
	"E32cSkYXC2bciKJGxRsE8ZgwmH/YwkvpD6/O9guSOa2irCwciH4xPHaH",
	"QtqgfMgHP2TFl",
	"zOKgHT56L7nIzFzDpUGhpWFrgP53m3G6ML",
	"S",
	"THxpsktzfFXizUv7DK1y/N7NZ1WhayViluBEvAJJ8bA1Wr6",
	"y9PXH3xGUhG/zQI8CaapRw2LhldCaFM9CRlKpZXJvj+pifu",
	"+RaaG7T8FRTI4cP019N5y9ofLyHE9ySFUr",
	"6Pf1l8UTeuzYldGtb/d",
}

type Client struct {
	Account      config.Account
	AccessToken  string
	RefreshToken string
	CaptchaToken string
	UserID       string
	ExpiresAt    int64
	DeviceID     string
	httpClient   *http.Client
	sessionDir   string
}

func New(account config.Account, sessionDir string) *Client {
	sum := md5.Sum([]byte(account.Username))
	return &Client{
		Account:    account,
		DeviceID:   hex.EncodeToString(sum[:]),
		httpClient: defaultAPIClient,
		sessionDir: sessionDir,
	}
}

func (c *Client) Login(ctx context.Context) error {
	if data, err := session.Load(c.sessionDir, c.Account.Username); err == nil {
		c.AccessToken = data.AccessToken
		c.RefreshToken = data.RefreshToken
		c.CaptchaToken = data.CaptchaToken
		c.UserID = data.UserID
		c.ExpiresAt = data.ExpiresAt
		if !data.Expired(time.Now()) {
			return nil
		}
		if c.RefreshToken != "" {
			if err := c.refreshToken(ctx); err == nil {
				return c.saveSession()
			}
		}
	}
	if err := c.passwordLogin(ctx); err != nil {
		return err
	}
	return c.saveSession()
}

func (c *Client) saveSession() error {
	return session.Save(c.sessionDir, c.Account.Username, &session.Data{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		CaptchaToken: c.CaptchaToken,
		UserID:       c.UserID,
		ExpiresAt:    c.ExpiresAt,
	})
}

func (c *Client) passwordLogin(ctx context.Context) error {
	token, err := c.loginCaptcha(ctx)
	if err != nil {
		return err
	}
	body := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"grant_type":    "password",
		"username":      c.Account.Username,
		"password":      c.Account.Password,
		"captcha_token": token,
	}
	var resp map[string]any
	if err := c.rawJSON(ctx, http.MethodPost, apiUser+"/v1/auth/signin", body, &resp); err != nil {
		return err
	}
	if code := number(resp["error_code"]); code != 0 {
		return fmt.Errorf("login failed: %s", respErrMessage(resp))
	}
	c.AccessToken = stringValue(resp["access_token"])
	c.RefreshToken = stringValue(resp["refresh_token"])
	c.UserID = stringValue(resp["sub"])
	c.CaptchaToken = token
	expiresIn := int64(number(resp["expires_in"]))
	c.ExpiresAt = time.Now().Add(time.Duration(expiresIn)*time.Second - session.ExpirySkew).Unix()
	return nil
}

func (c *Client) loginCaptcha(ctx context.Context) (string, error) {
	body := map[string]any{
		"client_id": clientID,
		"device_id": c.DeviceID,
		"action":    "POST:" + "/v1/auth/signin",
		"meta": map[string]string{
			"username": c.Account.Username,
		},
	}
	var resp map[string]any
	if err := c.rawJSON(ctx, http.MethodPost, apiUser+"/v1/shield/captcha/init", body, &resp); err != nil {
		return "", err
	}
	if code := number(resp["error_code"]); code != 0 {
		return "", fmt.Errorf("captcha init failed: %s", respErrMessage(resp))
	}
	verifyURL := stringValue(resp["url"])
	if verifyURL != "" {
		return "", fmt.Errorf("captcha verification required: %s", verifyURL)
	}
	return stringValue(resp["captcha_token"]), nil
}

func (c *Client) refreshToken(ctx context.Context) error {
	body := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": c.RefreshToken,
	}
	var resp map[string]any
	if err := c.rawJSON(ctx, http.MethodPost, apiUser+"/v1/auth/token", body, &resp); err != nil {
		return err
	}
	if code := number(resp["error_code"]); code != 0 {
		return fmt.Errorf("refresh token failed: %s", respErrMessage(resp))
	}
	if at := stringValue(resp["access_token"]); at != "" {
		c.AccessToken = at
	}
	if rt := stringValue(resp["refresh_token"]); rt != "" {
		c.RefreshToken = rt
	}
	if sub := stringValue(resp["sub"]); sub != "" {
		c.UserID = sub
	}
	expiresIn := int64(number(resp["expires_in"]))
	c.ExpiresAt = time.Now().Add(time.Duration(expiresIn)*time.Second - session.ExpirySkew).Unix()
	return nil
}

func (c *Client) Quota(ctx context.Context) (*QuotaMessage, error) {
	var quota QuotaMessage
	if err := c.doJSON(ctx, http.MethodGet, apiDrive+"/drive/v1/about", nil, &quota); err != nil {
		return nil, err
	}
	return &quota, nil
}

func (c *Client) ListFiles(ctx context.Context, remotePath string) ([]FileStat, error) {
	parentID := ""
	if cleanPath(remotePath) != "/" {
		if IsFileID(remotePath) {
			parentID = remotePath
		} else {
			id, err := c.FolderIDByPath(ctx, remotePath)
			if err != nil {
				return nil, err
			}
			parentID = id
		}
	}
	return c.ListByParentID(ctx, parentID)
}

func (c *Client) ListByParentID(ctx context.Context, parentID string) ([]FileStat, error) {
	values := url.Values{}
	values.Set("thumbnail_size", "SIZE_MEDIUM")
	values.Set("limit", defaultListLimit)
	values.Set("parent_id", parentID)
	values.Set("with_audit", "false")
	values.Set("filters", `{"trashed":{"eq":false}}`)
	var out []FileStat
	for {
		var resp filesResp
		u := apiDrive + "/drive/v1/files?" + values.Encode()
		if err := c.doJSON(ctx, http.MethodGet, u, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Files...)
		if resp.NextPageToken == "" {
			break
		}
		values.Set("page_token", resp.NextPageToken)
	}
	return out, nil
}

func (c *Client) FolderIDByPath(ctx context.Context, remotePath string) (string, error) {
	parts := splitRemotePath(remotePath)
	parentID := ""
	for _, part := range parts {
		files, err := c.ListByParentID(ctx, parentID)
		if err != nil {
			return "", err
		}
		found := ""
		for _, f := range files {
			if f.Kind == FileKindFolder && f.Name == part && !f.Trashed {
				found = f.ID
				break
			}
		}
		if found == "" {
			return "", fmt.Errorf("folder not found: %s", remotePath)
		}
		parentID = found
	}
	return parentID, nil
}

func (c *Client) FileByPath(ctx context.Context, remotePath string) (*File, error) {
	remotePath = cleanPath(remotePath)
	parent := path.Dir(remotePath)
	name := path.Base(remotePath)
	parentID := ""
	if parent != "/" && parent != "." {
		id, err := c.FolderIDByPath(ctx, parent)
		if err != nil {
			return nil, err
		}
		parentID = id
	}
	files, err := c.ListByParentID(ctx, parentID)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Name == name && !f.Trashed {
			return c.File(ctx, f.ID)
		}
	}
	return nil, fmt.Errorf("file not found: %s", remotePath)
}

func (c *Client) File(ctx context.Context, fileID string) (*File, error) {
	values := url.Values{}
	values.Set("thumbnail_size", "SIZE_LARGE")
	values.Set("usage", "FETCH")
	var f File
	u := apiDrive + "/drive/v1/files/" + url.PathEscape(fileID) + "?" + values.Encode()
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (c *Client) DeleteFiles(ctx context.Context, ids []string, force bool) error {
	body := map[string][]string{
		"ids": ids,
	}
	u := apiDrive + "/drive/v1/files:batchTrash"
	if force {
		u = apiDrive + "/drive/v1/files:batchDelete"
	}
	return c.doJSON(ctx, http.MethodPost, u, body, nil)
}

func (c *Client) EmptyTrash(ctx context.Context) error {
	u := apiDrive + "/drive/v1/files/trash:empty"
	return c.doJSON(ctx, http.MethodPatch, u, nil, nil)
}

func (c *Client) Download(ctx context.Context, target, output string, opts DownloadOptions) error {
	var f *File
	var err error
	if IsFileID(target) {
		f, err = c.File(ctx, target)
	} else {
		f, err = c.FileByPath(ctx, target)
	}
	if err != nil {
		return err
	}
	if f.Kind == FileKindFolder {
		return c.downloadFolder(ctx, f, output, opts)
	}
	return c.downloadFile(ctx, f, output, opts)
}

func (c *Client) downloadFile(ctx context.Context, f *File, output string, opts DownloadOptions) error {
	link := f.WebContentLink
	if f.Links.ApplicationOctetStream.URL != "" {
		link = f.Links.ApplicationOctetStream.URL
	}
	if link == "" && len(f.Medias) > 0 {
		link = f.Medias[0].Link.URL
	}
	if link == "" {
		return fmt.Errorf("download link not found for %s", f.Name)
	}
	return downloadURL(ctx, link, f, output, opts)
}

// downloadFolder downloads the direct child files of a folder. Subfolders are
// skipped (use a per-subfolder download for those).
func (c *Client) downloadFolder(ctx context.Context, f *File, output string, opts DownloadOptions) error {
	children, err := c.ListByParentID(ctx, f.ID)
	if err != nil {
		return err
	}
	var files []FileStat
	for _, ch := range children {
		if ch.Kind == fileKindFile && !ch.Trashed {
			files = append(files, ch)
		}
	}

	destDir := sanitizeFilename(f.Name)
	if output != "" {
		destDir = filepath.Join(output, destDir)
	}
	if len(files) == 0 {
		fmt.Printf("no files to download in folder: %s\n", f.Name)
		return nil
	}
	fmt.Printf("downloading folder %s: %d file(s) into %s\n", f.Name, len(files), destDir)

	var errs []error
	for i, ch := range files {
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(files), ch.Name)
		full, err := c.File(ctx, ch.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", ch.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", ch.Name, err))
			continue
		}
		if err := c.downloadFile(ctx, full, destDir+string(os.PathSeparator), opts); err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			fmt.Fprintf(os.Stderr, "error downloading %s: %v\n", ch.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", ch.Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to download %d of %d files", len(errs), len(files))
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, rawURL string, body any, out any) error {
	if err := c.ensureAccess(ctx); err != nil {
		return err
	}
	return c.doJSONAttempt(ctx, method, rawURL, body, out, true)
}

func (c *Client) doJSONAttempt(ctx context.Context, method, rawURL string, body any, out any, retry bool) error {
	reqBody, err := encodeBody(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reqBody)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var e errResp
	_ = json.Unmarshal(bs, &e)
	if e.IsError() {
		switch e.ErrorCode {
		case 4122, 4121, 16:
			if retry {
				if err := c.refreshToken(ctx); err != nil {
					return err
				}
				_ = c.saveSession()
				return c.doJSONAttempt(ctx, method, rawURL, body, out, false)
			}
		case 9:
			if retry {
				if err := c.refreshCaptcha(ctx, method, rawURL); err != nil {
					return err
				}
				_ = c.saveSession()
				return c.doJSONAttempt(ctx, method, rawURL, body, out, false)
			}
		}
		msg := joinErr(e.Error, e.ErrorDescription)
		return fmt.Errorf("pikpak api error %d: %s", e.ErrorCode, msg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http error %s: %s", resp.Status, string(bs))
	}
	if out != nil {
		if err := json.Unmarshal(bs, out); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) rawJSON(ctx context.Context, method, rawURL string, body any, out any) error {
	reqBody, err := encodeBody(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reqBody)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http error %s: %s", resp.Status, string(bs))
	}
	if out != nil {
		return json.Unmarshal(bs, out)
	}
	return nil
}

func (c *Client) ensureAccess(ctx context.Context) error {
	if c.AccessToken == "" || time.Now().Unix() >= c.ExpiresAt {
		if c.RefreshToken != "" {
			if err := c.refreshToken(ctx); err == nil {
				return c.saveSession()
			}
		}
		if err := c.passwordLogin(ctx); err != nil {
			return err
		}
		return c.saveSession()
	}
	return nil
}

func (c *Client) refreshCaptcha(ctx context.Context, method, rawURL string) error {
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	sign := clientID + clientVersion + packageName + c.DeviceID + ts
	for _, salt := range captchaSalts {
		sum := md5.Sum([]byte(sign + salt))
		sign = fmt.Sprintf("%x", sum)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", rawURL, err)
	}
	body := map[string]any{
		"action":        method + ":" + u.Path,
		"captcha_token": c.CaptchaToken,
		"client_id":     clientID,
		"device_id":     c.DeviceID,
		"meta": map[string]string{
			"captcha_sign":   "1." + sign,
			"user_id":        c.UserID,
			"package_name":   packageName,
			"client_version": clientVersion,
			"timestamp":      ts,
		},
		"redirect_uri": "xlaccsdk01://xbase.cloud/callback?state=harbor",
	}
	var resp map[string]any
	if err := c.rawJSON(ctx, http.MethodPost, apiUser+"/v1/shield/captcha/init?client_id="+clientID, body, &resp); err != nil {
		return err
	}
	if code := number(resp["error_code"]); code != 0 {
		return fmt.Errorf("captcha refresh failed: %s", respErrMessage(resp))
	}
	verifyURL := stringValue(resp["url"])
	if verifyURL != "" {
		return fmt.Errorf("captcha verification required: %s", verifyURL)
	}
	c.CaptchaToken = stringValue(resp["captcha_token"])
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Device-Id", c.DeviceID)
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}
	if c.CaptchaToken != "" {
		req.Header.Set("X-Captcha-Token", c.CaptchaToken)
	}
}

func encodeBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}
	bs, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(bs), nil
}

func cleanPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "/"
	}
	return path.Clean(p)
}

// IsFileID reports whether s matches the PikPak file/folder ID format,
// e.g. "VOw7XmbR7CNXy-Fk9WWu7cQho2" (26 characters of [A-Za-z0-9_-]).
func IsFileID(s string) bool {
	if len(s) != 26 {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func splitRemotePath(p string) []string {
	p = strings.Trim(cleanPath(p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		n, _ := strconv.ParseFloat(x, 64)
		return n
	default:
		return 0
	}
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// respErrMessage merges the error and error_description fields from an OAuth
// error response into a single human-readable string.
func respErrMessage(resp map[string]any) string {
	return joinErr(stringValue(resp["error"]), stringValue(resp["error_description"]))
}

func joinErr(err, desc string) string {
	if desc == "" {
		return err
	}
	if err == "" {
		return desc
	}
	return err + ": " + desc
}
