package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type findClient interface {
	Search(keyword, path string) ([]cloud115.Entry, error)
	Close() error
}

type findOpts struct {
	Out       io.Writer
	Keyword   string
	Path      string
	GetClient func() (findClient, error)
}

func newFindCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find <keyword> [path]",
		Short: "在 115 网盘中搜索文件",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/"
			if len(args) > 1 {
				path = args[1]
			}
			return findRun(&findOpts{
				Out:     cmd.OutOrStdout(),
				Keyword: args[0],
				Path:    path,
				GetClient: func() (findClient, error) {
					return getClient()
				},
			})
		},
	}
	return cmd
}

func findRun(o *findOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	results, err := client.Search(o.Keyword, o.Path)
	if err != nil {
		return fmt.Errorf("搜索失败: %w", err)
	}

	for _, r := range results {
		fmt.Fprintln(o.Out, r.Name)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newFindCmd())
}
