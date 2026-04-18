package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type statClient interface {
	Stat(path string) (*cloud115.Entry, error)
	Close() error
}

type statOpts struct {
	Out       io.Writer
	Path      string
	GetClient func() (statClient, error)
}

func newStatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stat <path>",
		Short: "显示文件或目录的元数据（JSON 格式）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return statRun(&statOpts{
				Out:  cmd.OutOrStdout(),
				Path: args[0],
				GetClient: func() (statClient, error) {
					return getClient()
				},
			})
		},
	}
	return cmd
}

func statRun(o *statOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	entry, err := client.Stat(o.Path)
	if err != nil {
		return fmt.Errorf("路径不存在: %s", o.Path)
	}

	enc := json.NewEncoder(o.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(entry)
}

func init() {
	rootCmd.AddCommand(newStatCmd())
}
