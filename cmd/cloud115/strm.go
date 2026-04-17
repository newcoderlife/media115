package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

var strmCmd = &cobra.Command{
	Use:   "strm PATH",
	Short: "Generate .strm files for Jellyfin from the tree cache",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := strings.Trim(args[0], "/")

		cfg, _ := getConfig()
		host := strmHost
		if host == "" {
			host = cfg.Proxy.Host
		}
		port := strmPort
		if port == 0 {
			port = cfg.Proxy.Port
		}
		baseURL := fmt.Sprintf("http://%s:%d", host, port)

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		entries, err := client.TreeEntries("")
		if err != nil {
			return err
		}

		// Filter video entries under the requested path.
		var matched []cloud115.TreeEntry
		for _, e := range entries {
			if e.IsVideo && (strings.HasPrefix(e.Path, path+"/") || e.Path == path) {
				matched = append(matched, e)
			}
		}

		if len(matched) == 0 {
			fmt.Printf("No video files found under '%s'\n", path)
			return nil
		}

		// Generate .strm files.
		created := 0
		for _, item := range matched {
			relPath := item.Path
			if strings.HasPrefix(relPath, path+"/") {
				relPath = relPath[len(path)+1:]
			}

			strmFile := filepath.Join(strmOutput, relPath+".strm")
			if err := os.MkdirAll(filepath.Dir(strmFile), 0o755); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}

			// URL-encode path components but keep / literal.
			encodedPath := url.PathEscape(item.Path)
			encodedPath = strings.ReplaceAll(encodedPath, "%2F", "/")

			strmURL := fmt.Sprintf("%s/redirect/%s\n", baseURL, encodedPath)
			if err := os.WriteFile(strmFile, []byte(strmURL), 0o644); err != nil {
				return fmt.Errorf("write strm: %w", err)
			}
			created++
		}

		fmt.Printf("Generated %d .strm files in %s\n", created, strmOutput)
		return nil
	},
}

var (
	strmOutput string
	strmHost   string
	strmPort   int
)

func init() {
	strmCmd.Flags().StringVarP(&strmOutput, "output", "o", "", "Output directory for .strm files (required)")
	_ = strmCmd.MarkFlagRequired("output")
	strmCmd.Flags().StringVar(&strmHost, "host", "", "Proxy host (default: from config)")
	strmCmd.Flags().IntVar(&strmPort, "port", 0, "Proxy port (default: from config)")
	rootCmd.AddCommand(strmCmd)
}
