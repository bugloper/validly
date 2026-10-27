package disposable

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newDetector(t *testing.T, cfg Config) *Detector {
	t.Helper()
	d, err := New(cfg, NewMemoryStore(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestBundledList(t *testing.T) {
	d := newDetector(t, Config{})
	if d.BundledCount() < 1000 {
		t.Fatalf("bundled list too small: %d", d.BundledCount())
	}
	for domain, want := range map[string]Status{
		"mailinator.com":       StatusDisposable,
		"inbox.mailinator.com": StatusDisposable,
		"yopmail.com":          StatusDisposable,
		"gmail.com":            StatusNotDisposable,
		"example.com":          StatusNotDisposable,
		"com":                  StatusNotDisposable,
	} {
		if got := d.Check(domain); got != want {
			t.Errorf("Check(%s) = %s, want %s", domain, got, want)
		}
	}
}

func TestAdminListsTakePrecedence(t *testing.T) {
	ctx := context.Background()
	d := newDetector(t, Config{})

	if _, err := d.AddEntry(ctx, Blocklist, "Corp-Throwaway.EXAMPLE"); err != nil {
		t.Fatal(err)
	}
	if got := d.Check("corp-throwaway.example"); got != StatusDisposable {
		t.Fatalf("blocklisted domain: %s", got)
	}
	if got := d.Check("sub.corp-throwaway.example"); got != StatusDisposable {
		t.Fatalf("blocklisted subdomain: %s", got)
	}

	// The allowlist overrides the bundled list, including for subdomains.
	if _, err := d.AddEntry(ctx, Allowlist, "mailinator.com"); err != nil {
		t.Fatal(err)
	}
	if got := d.Check("x.mailinator.com"); got != StatusNotDisposable {
		t.Fatalf("allowlisted parent: %s", got)
	}

	removed, err := d.RemoveEntry(ctx, Allowlist, "mailinator.com")
	if err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if got := d.Check("mailinator.com"); got != StatusDisposable {
		t.Fatalf("after removal: %s", got)
	}

	if _, err := d.AddEntry(ctx, Blocklist, "not a domain"); err != ErrInvalidDomain {
		t.Fatalf("expected ErrInvalidDomain, got %v", err)
	}
}

func TestFileReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extra.txt")
	if err := os.WriteFile(path, []byte("# custom\nthrowaway.example.org\n  bad domain  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := newDetector(t, Config{File: path})
	if d.Check("throwaway.example.org") != StatusDisposable {
		t.Fatal("file domain not loaded")
	}

	if err := os.WriteFile(path, []byte("other.example.org\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(path, future, future)
	if err := d.reloadFile(); err != nil {
		t.Fatal(err)
	}
	if d.Check("throwaway.example.org") != StatusNotDisposable || d.Check("other.example.org") != StatusDisposable {
		t.Fatal("file not reloaded")
	}

	if _, err := New(Config{File: filepath.Join(t.TempDir(), "missing")}, NewMemoryStore(), nil); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSync(t *testing.T) {
	body := "synced.example.net\n"
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	ctx := context.Background()
	d := newDetector(t, Config{SyncURL: srv.URL})
	if err := d.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if d.Check("synced.example.net") != StatusDisposable {
		t.Fatal("synced domain missing")
	}

	// A failed or empty sync keeps the previous list.
	status = http.StatusInternalServerError
	if err := d.Sync(ctx); err == nil {
		t.Fatal("expected error")
	}
	status, body = http.StatusOK, "# nothing\n"
	if err := d.Sync(ctx); err == nil {
		t.Fatal("expected error for empty list")
	}
	if d.Check("synced.example.net") != StatusDisposable {
		t.Fatal("previous list discarded")
	}

	body = strings.Repeat("a", maxSyncBytes+10)
	if err := d.Sync(ctx); err == nil {
		t.Fatal("expected size limit error")
	}

	// A line too long to scan must fail the sync, not truncate the list.
	body = "first.example.net\n" + strings.Repeat("x", 100_000) + "\nlast.example.net\n"
	if err := d.Sync(ctx); err == nil {
		t.Fatal("expected an error for an over-long line")
	}
	if d.Check("synced.example.net") != StatusDisposable || d.Check("first.example.net") != StatusNotDisposable {
		t.Fatal("a failed sync replaced the previous list")
	}
}
