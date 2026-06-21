package pikpak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultParallel = 4
	defaultChunkMin = 32 * 1024 * 1024
	copyBufferSize  = 512 * 1024
	maxRetries      = 3
	retryBaseDelay  = 300 * time.Millisecond
	tmpSuffix       = ".download"
	metaSuffix      = ".download.meta"
)

type DownloadOptions struct {
	Parallel int
	ChunkMin int64
	Force    bool
}

func (o DownloadOptions) parallel() int {
	if o.Parallel <= 0 {
		return defaultParallel
	}
	return o.Parallel
}

func (o DownloadOptions) chunkMin() int64 {
	if o.ChunkMin <= 0 {
		return defaultChunkMin
	}
	return o.ChunkMin
}

var (
	errRangeUnsupported     = errors.New("server does not support range requests")
	errLocalPartialMismatch = errors.New("local file exists with mismatched size")
)

var downloadHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConnsPerHost:   16,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

func downloadURL(ctx context.Context, link string, f *File, output string, opts DownloadOptions) error {
	name := f.Name
	if name == "" {
		name = f.ID
	}
	name = sanitizeFilename(name)
	dest := output
	if dest == "" {
		dest = name
	} else if info, err := os.Stat(dest); err == nil && info.IsDir() {
		dest = filepath.Join(dest, name)
	} else if strings.HasSuffix(dest, string(os.PathSeparator)) {
		dest = filepath.Join(dest, name)
	}
	expected := f.Size

	if opts.Force {
		_ = os.Remove(dest)
		_ = os.Remove(dest + tmpSuffix)
		_ = os.Remove(dest + metaSuffix)
	} else if expected > 0 {
		if info, err := os.Stat(dest); err == nil && info.Size() == expected {
			_ = os.Remove(dest + tmpSuffix)
			_ = os.Remove(dest + metaSuffix)
			fmt.Printf("already complete: %s (size matches; use --force to redownload)\n", dest)
			return nil
		}
	}

	var lastErr error
	parallelN := opts.parallel()
	chunkMin := opts.chunkMin()
	if expected >= chunkMin && parallelN >= 2 {
		err := parallelDownload(ctx, link, dest, expected, parallelN, chunkMin)
		if err == nil {
			fmt.Printf("downloaded: %s\n", dest)
			return nil
		}
		// User cancelled (Ctrl+C) or deadline expired: surface immediately
		// rather than falling through to single-connection, which would create
		// a 0-byte dest file before its own HTTP request also fails.
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		switch {
		case errors.Is(err, errRangeUnsupported):
			fmt.Fprintln(os.Stderr, "note: server does not support range; falling back to single connection")
		case errors.Is(err, errLocalPartialMismatch):
			fmt.Fprintln(os.Stderr, "note: local file exists with different size; falling back to single connection")
		default:
			lastErr = err
		}
	}
	for attempt := 0; attempt < maxRetries; attempt++ {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		err := downloadAttempt(ctx, link, dest, expected)
		if err == nil {
			fmt.Printf("downloaded: %s\n", dest)
			return nil
		}
		lastErr = err
		if attempt == maxRetries-1 {
			break
		}
		if err := sleepCtx(ctx, time.Duration(attempt+1)*retryBaseDelay); err != nil {
			return err
		}
	}
	return lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func downloadAttempt(ctx context.Context, link, dest string, expected int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOrClean(dest)), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	info, err := out.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if expected > 0 && offset > expected {
		if err := out.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}
	if expected > 0 && offset == expected {
		p := newProgressPrinter(expected, offset)
		p.finish()
		return nil
	}
	if _, err := out.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if offset > 0 && resp.StatusCode == http.StatusOK {
		if err := out.Truncate(0); err != nil {
			return err
		}
		return fmt.Errorf("server ignored range request; restarted download")
	}
	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("download request failed: %s", resp.Status)
	}
	if offset == 0 && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return fmt.Errorf("download request failed: %s", resp.Status)
	}
	p := newProgressPrinter(expected, offset)
	defer p.finish()
	written, err := copyWithProgress(out, resp.Body, p)
	if err != nil {
		return err
	}
	if expected > 0 && offset+written != expected {
		return fmt.Errorf("download incomplete: got %d of %d bytes", offset+written, expected)
	}
	return nil
}

type partRange struct {
	start int64
	end   int64
	done  int64
}

func (p *partRange) length() int64 { return p.end - p.start + 1 }

// downloadMeta is persisted alongside the .download file so that a partial
// parallel download can resume across process restarts.
type downloadMeta struct {
	Expected int64      `json:"expected"`
	Parts    []partMeta `json:"parts"`
}

type partMeta struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
}

func snapshotParts(parts []*partRange, expected int64) *downloadMeta {
	m := &downloadMeta{Expected: expected, Parts: make([]partMeta, len(parts))}
	for i, p := range parts {
		m.Parts[i] = partMeta{Start: p.start, End: p.end, Done: p.done}
	}
	return m
}

func restoreParts(m *downloadMeta) []*partRange {
	out := make([]*partRange, len(m.Parts))
	for i, p := range m.Parts {
		out[i] = &partRange{start: p.Start, end: p.End, done: p.Done}
	}
	return out
}

func validMeta(m *downloadMeta, expected int64) bool {
	if m == nil || m.Expected != expected || len(m.Parts) == 0 {
		return false
	}
	var cursor int64
	for _, p := range m.Parts {
		length := p.End - p.Start + 1
		if p.Start != cursor || length <= 0 || p.Done < 0 || p.Done > length {
			return false
		}
		cursor = p.End + 1
	}
	return cursor == expected
}

func saveMeta(path string, m *downloadMeta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func loadMeta(path string) (*downloadMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m downloadMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// parallelDownload pre-allocates a single .download file alongside a sidecar
// .download.meta that records per-range progress. If both exist with a matching
// expected size on entry, the download resumes; otherwise it starts fresh.
// On success the temp file is renamed onto dest and the meta is removed; on
// errRangeUnsupported both are removed so the single-connection fallback can
// run; on any other failure (including Ctrl+C via context cancellation) both
// are preserved so the next process can resume.
//
// Progress is persisted to .meta only on exit via the deferred saveMeta — no
// periodic background writes. Ctrl+C triggers signal.NotifyContext in main,
// which cancels ctx and lets the defer chain run normally.
func parallelDownload(ctx context.Context, link, dest string, expected int64, parallelN int, chunkMin int64) (err error) {
	if expected <= 0 {
		return errRangeUnsupported
	}
	if info, statErr := os.Stat(dest); statErr == nil {
		if info.Size() > 0 {
			return errLocalPartialMismatch
		}
		_ = os.Remove(dest)
	}
	if err = os.MkdirAll(filepath.Dir(absOrClean(dest)), 0755); err != nil {
		return err
	}

	tmpPath := dest + tmpSuffix
	metaPath := dest + metaSuffix

	parts, file, resumed, err := openDownloadFile(tmpPath, metaPath, expected, parallelN, chunkMin)
	if err != nil {
		return err
	}
	if resumed {
		var done int64
		for _, p := range parts {
			done += p.done
		}
		fmt.Fprintf(os.Stderr, "resuming parallel download: %s / %s already on disk\n", ByteSize(done), ByteSize(expected))
	}

	fileClosed := false
	purge := false     // errRangeUnsupported: remove tmpPath+metaPath
	succeeded := false // rename succeeded: meta already removed, nothing to save
	defer func() {
		if !fileClosed {
			file.Close()
		}
		if purge {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaPath)
			return
		}
		if !succeeded {
			// Persist progress so the next process can resume.
			_ = saveMeta(metaPath, snapshotParts(parts, expected))
		}
	}()

	var attemptErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		attemptErr = parallelAttempt(ctx, link, file, parts, expected)

		if attemptErr == nil {
			if err = file.Close(); err != nil {
				return err
			}
			fileClosed = true
			if err = os.Remove(dest); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err = os.Rename(tmpPath, dest); err != nil {
				return err
			}
			_ = os.Remove(metaPath)
			succeeded = true
			return nil
		}
		if errors.Is(attemptErr, errRangeUnsupported) {
			purge = true
			return errRangeUnsupported
		}
		if attempt == maxRetries-1 {
			break
		}
		if err = sleepCtx(ctx, time.Duration(attempt+1)*retryBaseDelay); err != nil {
			return err
		}
	}
	return attemptErr
}

// openDownloadFile returns the open .download file and the part list, resuming
// from an existing meta sidecar when both the temp file and meta validate.
func openDownloadFile(tmpPath, metaPath string, expected int64, parallelN int, chunkMin int64) ([]*partRange, *os.File, bool, error) {
	if m, err := loadMeta(metaPath); err == nil && validMeta(m, expected) {
		if info, statErr := os.Stat(tmpPath); statErr == nil && info.Size() == expected {
			if f, openErr := os.OpenFile(tmpPath, os.O_RDWR, 0644); openErr == nil {
				return restoreParts(m), f, true, nil
			}
		}
	}
	// stale or absent meta: wipe both and start fresh
	_ = os.Remove(metaPath)
	parts, err := makePartRanges(expected, parallelN, chunkMin)
	if err != nil {
		return nil, nil, false, err
	}
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, nil, false, err
	}
	if err := file.Truncate(expected); err != nil {
		file.Close()
		_ = os.Remove(tmpPath)
		return nil, nil, false, err
	}
	return parts, file, false, nil
}

// makePartRanges splits expected bytes into N >= 2 contiguous ranges. Returns
// errRangeUnsupported if parallelism cannot apply.
func makePartRanges(expected int64, parallelN int, chunkMin int64) ([]*partRange, error) {
	count := parallelN
	if count < 2 {
		return nil, errRangeUnsupported
	}
	if chunkMin <= 0 {
		chunkMin = defaultChunkMin
	}
	if expected < int64(count)*chunkMin {
		count = int((expected + chunkMin - 1) / chunkMin)
	}
	if count < 2 {
		return nil, errRangeUnsupported
	}
	parts := make([]*partRange, 0, count)
	partSize := (expected + int64(count) - 1) / int64(count)
	for i := 0; i < count; i++ {
		start := int64(i) * partSize
		end := start + partSize - 1
		if end >= expected {
			end = expected - 1
		}
		if start > end {
			break
		}
		parts = append(parts, &partRange{start: start, end: end})
	}
	return parts, nil
}

// parallelAttempt runs one pass over all unfinished ranges, writing directly
// into file via WriteAt. Each goroutine updates its part.done so the next
// attempt resumes from where this one left off.
func parallelAttempt(parentCtx context.Context, link string, file *os.File, parts []*partRange, expected int64) error {
	var initial int64
	for _, part := range parts {
		initial += part.done
	}
	p := newProgressPrinter(expected, initial)
	defer p.finish()

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	errCh := make(chan error, len(parts))
	var wg sync.WaitGroup
	for _, part := range parts {
		if part.done >= part.length() {
			continue
		}
		part := part
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := downloadPartTo(ctx, link, file, part, p); err != nil {
				cancel()
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	// If the caller's context was canceled, surface that rather than treating
	// the (filtered-out) context.Canceled child errors as a silent success.
	if err := parentCtx.Err(); err != nil {
		return err
	}

	var firstErr error
	rangeUnsupported := false
	for err := range errCh {
		if errors.Is(err, errRangeUnsupported) {
			rangeUnsupported = true
			continue
		}
		if errors.Is(err, context.Canceled) {
			continue
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if rangeUnsupported {
		return errRangeUnsupported
	}
	return firstErr
}

// downloadPartTo fetches one range and writes it at the absolute byte offset
// via WriteAt (goroutine-safe per os.File docs). Resumes mid-range from
// part.start + part.done.
func downloadPartTo(ctx context.Context, link string, file *os.File, part *partRange, p *progressPrinter) error {
	rangeStart := part.start + part.done
	if rangeStart > part.end {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, part.end))
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return errRangeUnsupported
	}
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("range download request failed: %s", resp.Status)
	}

	buf := make([]byte, copyBufferSize)
	offset := rangeStart
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := file.WriteAt(buf[:n], offset); werr != nil {
				return werr
			}
			offset += int64(n)
			part.done += int64(n)
			p.add(int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if part.done != part.length() {
		return fmt.Errorf("part download incomplete: got %d of %d bytes", part.done, part.length())
	}
	return nil
}

func copyWithProgress(dst io.Writer, src io.Reader, p *progressPrinter) (int64, error) {
	return io.CopyBuffer(dst, &progressReader{reader: src, progress: p}, make([]byte, copyBufferSize))
}

type progressReader struct {
	reader   io.Reader
	progress *progressPrinter
}

func (r *progressReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	if n > 0 {
		r.progress.add(int64(n))
	}
	return n, err
}

type progressPrinter struct {
	mu             sync.Mutex
	total          int64
	done           int64
	started        time.Time
	lastRender     time.Time
	lastSampleAt   time.Time
	lastSampleDone int64
	emaSpeed       float64
	finished       bool
	maxVisible     int // longest visible width ever written; used to pad shorter lines so '\r' doesn't leave residual chars
}

func newProgressPrinter(total, initial int64) *progressPrinter {
	now := time.Now()
	p := &progressPrinter{
		total:          total,
		done:           initial,
		started:        now,
		lastSampleAt:   now,
		lastSampleDone: initial,
	}
	p.mu.Lock()
	line, _ := p.formatLocked(now, true)
	p.mu.Unlock()
	fmt.Print(line)
	return p
}

func (p *progressPrinter) add(n int64) {
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		return
	}
	p.done += n
	line, ok := p.formatLocked(time.Now(), false)
	p.mu.Unlock()
	if ok {
		fmt.Print(line)
	}
}

func (p *progressPrinter) finish() {
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		return
	}
	p.finished = true
	line, _ := p.formatLocked(time.Now(), true)
	p.mu.Unlock()
	fmt.Print(line)
	fmt.Println()
}

func (p *progressPrinter) formatLocked(now time.Time, force bool) (string, bool) {
	if !force && now.Sub(p.lastRender) < 250*time.Millisecond {
		return "", false
	}
	p.lastRender = now

	dt := now.Sub(p.lastSampleAt).Seconds()
	if dt >= 0.5 || force {
		dDone := p.done - p.lastSampleDone
		if dt > 0 {
			inst := float64(dDone) / dt
			if p.emaSpeed == 0 {
				p.emaSpeed = inst
			} else {
				p.emaSpeed = 0.3*inst + 0.7*p.emaSpeed
			}
		}
		p.lastSampleAt = now
		p.lastSampleDone = p.done
	}
	speed := int64(p.emaSpeed)
	if speed == 0 {
		elapsed := now.Sub(p.started).Seconds()
		if elapsed > 0 {
			speed = int64(float64(p.done) / elapsed)
		}
	}

	if p.total > 0 {
		percent := float64(p.done) * 100 / float64(p.total)
		if percent > 100 {
			percent = 100
		}
		return p.padLocked(fmt.Sprintf("\rprogress: %6.2f%%  %s/%s  %s/s", percent, ByteSize(p.done), ByteSize(p.total), ByteSize(speed))), true
	}
	return p.padLocked(fmt.Sprintf("\rprogress: %s  %s/s", ByteSize(p.done), ByteSize(speed))), true
}

// padLocked pads the rendered line with trailing spaces so each successive
// '\r'-prefixed write covers any longer previous line, preventing residual
// chars like "1.17MB/ssB/s" when the new line is shorter than the old one.
func (p *progressPrinter) padLocked(line string) string {
	visible := len(line) - 1 // exclude leading '\r'
	if visible > p.maxVisible {
		p.maxVisible = visible
		return line
	}
	if visible < p.maxVisible {
		return line + strings.Repeat(" ", p.maxVisible-visible)
	}
	return line
}

func ByteSize(n int64) string {
	if n < 0 {
		n = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(n)
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d%s", n, units[i])
	}
	return fmt.Sprintf("%.2f%s", value, units[i])
}

func absOrClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// sanitizeFilename replaces characters that are illegal on Windows (and path
// separators on any OS) with '_'. Trailing dots/spaces are also stripped
// because Windows silently drops them and the file would not be findable.
func sanitizeFilename(name string) string {
	const illegal = `<>:"/\|?*`
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(illegal, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimRight(b.String(), ". ")
	if out == "" || out == "." || out == ".." {
		return "_"
	}
	return out
}
