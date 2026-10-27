// Package disposable detects addresses at disposable (throwaway) email
// providers.
//
// Detection is a heuristic: new disposable services appear constantly and no
// list is complete. A "not_disposable" result only means the domain is not on
// any configured list.
//
// A domain is checked against, in order of precedence:
//  1. the administrator allowlist (never disposable),
//  2. the administrator blocklist,
//  3. the bundled list, an optional local file and an optional synced URL.
//
// Parent domains match too, so "x.mailinator.com" is disposable when
// "mailinator.com" is listed.
package disposable

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/validly/validly/internal/syntax"
)

//go:embed data/disposable_domains.txt
var bundled string

// Status is the outcome of a disposable check.
type Status string

const (
	StatusDisposable    Status = "disposable"
	StatusNotDisposable Status = "not_disposable"
)

// ListName identifies an administrator-managed list.
type ListName string

const (
	Blocklist ListName = "blocklist"
	Allowlist ListName = "allowlist"
)

// Valid reports whether l is a known list name.
func (l ListName) Valid() bool { return l == Blocklist || l == Allowlist }

// Entry is one administrator-managed domain.
type Entry struct {
	List      ListName  `json:"list"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminStore persists administrator-managed lists.
type AdminStore interface {
	ListEntries(ctx context.Context) ([]Entry, error)
	AddEntry(ctx context.Context, list ListName, domain string) error
	// RemoveEntry reports whether the entry existed.
	RemoveEntry(ctx context.Context, list ListName, domain string) (bool, error)
}

type domainSet map[string]struct{}

type adminSnapshot struct {
	block, allow domainSet
}

// Config configures a Detector.
type Config struct {
	// File is an optional path to a newline-separated domain list. It is
	// re-read when its modification time changes.
	File string
	// SyncURL is an optional URL serving a newline-separated domain list,
	// fetched every SyncInterval. It is configured by the operator, never by
	// API callers.
	SyncURL      string
	SyncInterval time.Duration
	// RefreshInterval controls how often the file and admin lists are
	// re-read so changes made on other replicas become visible.
	RefreshInterval time.Duration
	HTTPClient      *http.Client
}

// Detector checks domains against the configured lists. It is safe for
// concurrent use.
type Detector struct {
	cfg     Config
	store   AdminStore
	log     *slog.Logger
	bundled domainSet

	file   atomic.Pointer[domainSet]
	synced atomic.Pointer[domainSet]
	admin  atomic.Pointer[adminSnapshot]

	fileMu      sync.Mutex
	fileModTime time.Time
}

// New returns a Detector. store holds the admin lists; use NewMemoryStore
// when no database is configured.
func New(cfg Config, store AdminStore, log *slog.Logger) (*Detector, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 30 * time.Second
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 24 * time.Hour
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	set, _, err := parseList(strings.NewReader(bundled))
	if err != nil {
		return nil, fmt.Errorf("bundled disposable list: %w", err)
	}
	d := &Detector{cfg: cfg, store: store, log: log, bundled: set}
	d.admin.Store(&adminSnapshot{block: domainSet{}, allow: domainSet{}})
	if cfg.File != "" {
		if err := d.reloadFile(); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// BundledCount reports how many domains ship with the binary.
func (d *Detector) BundledCount() int { return len(d.bundled) }

// Check classifies domain, which must be a normalized ASCII domain.
func (d *Detector) Check(domain string) Status {
	admin := d.admin.Load()
	file, synced := d.file.Load(), d.synced.Load()

	disposable := false
	for s := domain; s != ""; s = parent(s) {
		if _, ok := admin.allow[s]; ok {
			return StatusNotDisposable
		}
		if disposable {
			continue
		}
		if contains(admin.block, s) || contains(d.bundled, s) || containsPtr(file, s) || containsPtr(synced, s) {
			// Keep walking: an allowlisted parent overrides a listed subdomain.
			disposable = true
		}
	}
	if disposable {
		return StatusDisposable
	}
	return StatusNotDisposable
}

func contains(s domainSet, d string) bool { _, ok := s[d]; return ok }

func containsPtr(s *domainSet, d string) bool { return s != nil && contains(*s, d) }

func parent(d string) string {
	i := strings.IndexByte(d, '.')
	if i < 0 {
		return ""
	}
	p := d[i+1:]
	if strings.IndexByte(p, '.') < 0 {
		return "" // never match a bare TLD
	}
	return p
}

// Run refreshes the admin lists, the file and the synced URL until ctx ends.
func (d *Detector) Run(ctx context.Context) {
	d.refreshAdmin(ctx)
	if d.cfg.SyncURL != "" {
		d.sync(ctx)
	}
	refresh := time.NewTicker(d.cfg.RefreshInterval)
	defer refresh.Stop()
	var syncC <-chan time.Time
	if d.cfg.SyncURL != "" {
		t := time.NewTicker(d.cfg.SyncInterval)
		defer t.Stop()
		syncC = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh.C:
			d.refreshAdmin(ctx)
			if d.cfg.File != "" {
				if err := d.reloadFile(); err != nil {
					d.log.WarnContext(ctx, "disposable file reload failed", "error", err)
				}
			}
		case <-syncC:
			d.sync(ctx)
		}
	}
}

// refreshAdmin reloads the admin snapshot from the store.
func (d *Detector) refreshAdmin(ctx context.Context) {
	if err := d.RefreshAdmin(ctx); err != nil {
		d.log.WarnContext(ctx, "disposable admin list refresh failed", "error", err)
	}
}

// RefreshAdmin reloads the administrator lists from the store.
func (d *Detector) RefreshAdmin(ctx context.Context) error {
	entries, err := d.store.ListEntries(ctx)
	if err != nil {
		return err
	}
	snap := &adminSnapshot{block: domainSet{}, allow: domainSet{}}
	for _, e := range entries {
		if e.List == Allowlist {
			snap.allow[e.Domain] = struct{}{}
		} else {
			snap.block[e.Domain] = struct{}{}
		}
	}
	d.admin.Store(snap)
	return nil
}

// ErrInvalidDomain is returned when an admin entry is not a valid domain.
var ErrInvalidDomain = errors.New("invalid domain")

// AddEntry normalizes domain, stores it and refreshes the local snapshot.
func (d *Detector) AddEntry(ctx context.Context, list ListName, domain string) (string, error) {
	norm, reason := syntax.NormalizeDomain(domain)
	if reason != "" {
		return "", ErrInvalidDomain
	}
	if err := d.store.AddEntry(ctx, list, norm); err != nil {
		return "", err
	}
	return norm, d.RefreshAdmin(ctx)
}

// RemoveEntry deletes domain from list and refreshes the local snapshot.
func (d *Detector) RemoveEntry(ctx context.Context, list ListName, domain string) (bool, error) {
	norm, reason := syntax.NormalizeDomain(domain)
	if reason != "" {
		return false, ErrInvalidDomain
	}
	ok, err := d.store.RemoveEntry(ctx, list, norm)
	if err != nil {
		return false, err
	}
	return ok, d.RefreshAdmin(ctx)
}

// Entries returns the administrator-managed entries.
func (d *Detector) Entries(ctx context.Context) ([]Entry, error) {
	return d.store.ListEntries(ctx)
}

func (d *Detector) reloadFile() error {
	d.fileMu.Lock()
	defer d.fileMu.Unlock()
	fi, err := os.Stat(d.cfg.File)
	if err != nil {
		return fmt.Errorf("disposable file: %w", err)
	}
	if fi.ModTime().Equal(d.fileModTime) && d.file.Load() != nil {
		return nil
	}
	f, err := os.Open(d.cfg.File)
	if err != nil {
		return fmt.Errorf("disposable file: %w", err)
	}
	defer f.Close()
	set, skipped, err := parseList(f)
	if err != nil {
		return fmt.Errorf("disposable file: %w", err)
	}
	d.file.Store(&set)
	d.fileModTime = fi.ModTime()
	d.log.Info("disposable file loaded", "domains", len(set), "skipped", skipped)
	return nil
}

// maxSyncBytes bounds the size of a synced list.
const maxSyncBytes = 16 << 20

func (d *Detector) sync(ctx context.Context) {
	if err := d.Sync(ctx); err != nil {
		d.log.WarnContext(ctx, "disposable list sync failed; keeping previous list", "error", err)
	}
}

// Sync fetches the configured SyncURL once. On failure the previous list is kept.
func (d *Detector) Sync(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.cfg.SyncURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Validly disposable-list sync")
	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body := io.LimitReader(resp.Body, maxSyncBytes+1)
	counter := &countingReader{r: body}
	set, skipped, err := parseList(counter)
	if counter.n > maxSyncBytes {
		return fmt.Errorf("list exceeds %d bytes", maxSyncBytes)
	}
	if err != nil {
		return err
	}
	if len(set) == 0 {
		return errors.New("synced list is empty")
	}
	d.synced.Store(&set)
	d.log.InfoContext(ctx, "disposable list synced", "domains", len(set), "skipped", skipped)
	return nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// parseList reads one domain per line, ignoring blanks and '#' comments.
// Invalid domains are skipped and counted. A read error, including a line
// too long to be a domain, fails the whole list so that a truncated list is
// never mistaken for a complete one.
func parseList(r io.Reader) (domainSet, int, error) {
	set := domainSet{}
	skipped := 0
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		norm, reason := syntax.NormalizeDomain(line)
		if reason != "" {
			skipped++
			continue
		}
		set[norm] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, skipped, err
	}
	return set, skipped, nil
}
