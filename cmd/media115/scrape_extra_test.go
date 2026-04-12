package main

import (
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/organizer"
)

// ── uploadMissingNFO edge cases ───────────────────────────────────────────────

// TestUploadMissingNFONoSkips verifies that an empty skip list is a no-op
// (does not panic, does not call any client methods).
func TestUploadMissingNFONoSkips(t *testing.T) {
	// A nil client is safe because uploadMissingNFO returns early when
	// len(skips) == 0.
	uploadMissingNFO(nil, "影音/AV", []organizer.Op{}, nil, nil)
}

// TestUploadMissingNFONilLogger verifies that a nil logger falls back to the
// default without panicking, when there are no skips.
func TestUploadMissingNFONilLoggerNoSkips(t *testing.T) {
	uploadMissingNFO(nil, "影音/电影", []organizer.Op{}, []cloud115.TreeEntry{}, nil)
}

// TestUploadMissingNFOSkipReasonFilter verifies that ops whose Reason is not
// "already correct" or "already in standard format" are silently ignored,
// so no client call is attempted (client is nil, would panic if called).
func TestUploadMissingNFOSkipReasonFilter(t *testing.T) {
	skips := []organizer.Op{
		{
			File:   "SomeMovie.2023.mkv",
			Parent: "影音/电影/SomeMovie.2023",
			Reason: "some other reason",
		},
		{
			File:   "Another.2020.mkv",
			Parent: "影音/电影/Another.2020",
			Reason: "unknown reason",
		},
	}
	// If the filter doesn't work, SyncSidecars would be called with a nil
	// client and would panic.
	uploadMissingNFO(nil, "影音/电影", skips, []cloud115.TreeEntry{}, nil)
}

// TestUploadMissingNFOSkipWhenNFOExists verifies that a skip op whose NFO
// already exists in the tree is correctly filtered out before any client call.
func TestUploadMissingNFOSkipWhenNFOExists(t *testing.T) {
	filename := "Good.Movie.2022.mkv"
	parent := "影音/电影/Good.Movie.2022"
	// Simulate an existing NFO in the tree for this file.
	treeEntries := []cloud115.TreeEntry{
		{
			Name:    "Good.Movie.2022.nfo",
			Path:    "影音/电影/Good.Movie.2022/Good.Movie.2022.nfo",
			Parent:  parent,
			IsNFO:   true,
			IsVideo: false,
		},
	}
	skips := []organizer.Op{
		{
			File:   filename,
			Parent: parent,
			Reason: "already correct",
		},
	}
	// The NFO exists in treeEntries, so the video should be filtered out
	// before any client call is made. Passing nil client is safe here.
	uploadMissingNFO(nil, "影音/电影", skips, treeEntries, nil)
}
