package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type strmClient interface {
	TreeEntries(category string) ([]cloud115.TreeEntry, error)
	Close() error
}

type strmOpts struct {
	Out       io.Writer
	Path      string
	Output    string
	Host      string
	Port      int
	GetClient func() (strmClient, error)
	WriteFile func(name string, data []byte, perm os.FileMode) error
	MkdirAll  func(path string, perm os.FileMode) error
}

func newStrmCmd() *cobra.Command {
	var (
		output string
		host   string
		port   int
	)
	cmd := &cobra.Command{
		Use:   "strm PATH",
		Short: "Generate .strm files for Jellyfin from the tree cache",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := getConfig()
			h := host
			if h == "" {
				h = cfg.Proxy.Host
			}
			p := port
			if p == 0 {
				p = cfg.Proxy.Port
			}
			return strmRun(&strmOpts{
				Out:    cmd.OutOrStdout(),
				Path:   strings.Trim(args[0], "/"),
				Output: output,
				Host:   h,
				Port:   p,
				GetClient: func() (strmClient, error) {
					return getClient()
				},
				WriteFile: os.WriteFile,
				MkdirAll:  os.MkdirAll,
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output directory for .strm files (required)")
	_ = cmd.MarkFlagRequired("output")
	cmd.Flags().StringVar(&host, "host", "", "Proxy host (default: from config)")
	cmd.Flags().IntVar(&port, "port", 0, "Proxy port (default: from config)")
	return cmd
}

func strmRun(o *strmOpts) error {
	baseURL := fmt.Sprintf("http://%s:%d", o.Host, o.Port)

	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	entries, err := client.TreeEntries("")
	if err != nil {
		return err
	}

	var matched []cloud115.TreeEntry
	for _, e := range entries {
		if e.IsVideo && (strings.HasPrefix(e.Path, o.Path+"/") || e.Path == o.Path) {
			matched = append(matched, e)
		}
	}

	if len(matched) == 0 {
		fmt.Fprintf(o.Out, "No video files found under '%s'\n", o.Path)
		return nil
	}

	created := 0
	for _, item := range matched {
		relPath := item.Path
		if strings.HasPrefix(relPath, o.Path+"/") {
			relPath = relPath[len(o.Path)+1:]
		}

		strmFile := filepath.Join(o.Output, relPath+".strm")
		if err := o.MkdirAll(filepath.Dir(strmFile), 0o755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}

		encodedPath := url.PathEscape(item.Path)
		encodedPath = strings.ReplaceAll(encodedPath, "%2F", "/")

		strmURL := fmt.Sprintf("%s/redirect/%s\n", baseURL, encodedPath)
		if err := o.WriteFile(strmFile, []byte(strmURL), 0o644); err != nil {
			return fmt.Errorf("write strm: %w", err)
		}
		created++
	}

	fmt.Fprintf(o.Out, "Generated %d .strm files in %s\n", created, o.Output)
	return nil
}

func init() {
	rootCmd.AddCommand(newStrmCmd())
}
