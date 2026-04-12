package main

import "testing"

// TestScrapeFixCmdHelp verifies that scrapeFixCmd has the expected flags
// registered at init time.
func TestScrapeFixCmdHelp(t *testing.T) {
	cmd := scrapeFixCmd

	flags := []string{"tmdb-id", "search", "number"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s flag on scrapeFixCmd", name)
		}
	}
}
