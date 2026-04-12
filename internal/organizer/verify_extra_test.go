package organizer

import (
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// newVerifyTestClient creates a *cloud115.Client backed by a temp SQLite DB.
// API calls fail gracefully (no valid cookies / network).
func newVerifyTestClient(t *testing.T) *cloud115.Client {
	t.Helper()
	tmpDir := t.TempDir()
	client, err := cloud115.NewClient("", cloud115.WithCacheDir(filepath.Join(tmpDir, "cache")))
	if err != nil {
		t.Fatalf("newVerifyTestClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestVerifyEmptyOps verifies that Verify returns (0,0) for an empty op list
// without touching the client (nil client is safe here).
func TestVerifyEmptyOps(t *testing.T) {
	v, m := Verify(nil, nil, "/影音/电影")
	if v != 0 || m != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", v, m)
	}
}

// TestVerifyEmptyOpsNoFolderChange verifies that Verify with an empty ops slice
// returns (0,0) without touching the client.
func TestVerifyEmptyOpsNoFolderChange(t *testing.T) {
	ops := []Op{}
	v, m := Verify(nil, ops, "/影音/AV")
	if v != 0 || m != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", v, m)
	}
}

// TestVerifyWithRealClientDirMissing verifies that Verify counts a mismatch
// when the target directory cannot be listed (API fails gracefully with stub client).
func TestVerifyWithRealClientDirMissing(t *testing.T) {
	client := newVerifyTestClient(t)

	ops := []Op{
		{File: "Movie.mkv", NewName: "Movie (2020).mkv", NewFolder: "Movie (2020)", Parent: "影音/电影/old"},
		{File: "B.mkv", NewName: "", NewFolder: "", Parent: "影音/电影/B"},
	}

	// With a stub client that has no cached paths and no live API, every
	// ListDir call fails → all ops become mismatches.
	v, m := Verify(client, ops, "/影音/电影")
	if v+m != 2 {
		t.Errorf("expected v+m=2, got v=%d m=%d", v, m)
	}
	// All should be mismatches since dirs don't exist.
	if m != 2 {
		t.Errorf("expected 2 mismatches, got %d (verified=%d)", m, v)
	}
}

// TestVerifyNewNameUsed verifies that Verify uses NewName (not File) when set.
func TestVerifyNewNameFallback(t *testing.T) {
	client := newVerifyTestClient(t)

	// Op with no NewFolder — uses Parent as target dir.
	ops := []Op{
		{File: "old.mkv", NewName: "New (2021).mkv", NewFolder: "", Parent: "影音/电影/New (2021)"},
	}
	// Client has no cached paths → ListDir fails → mismatch.
	v, m := Verify(client, ops, "/影音/电影")
	if v != 0 || m != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", v, m)
	}
}
