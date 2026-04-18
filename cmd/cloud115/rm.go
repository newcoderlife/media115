package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type rmClient interface {
	Stat(path string) (*cloud115.Entry, error)
	Delete(paths []string) error
	Close() error
}

type rmOpts struct {
	Out       io.Writer
	Paths     []string
	Recursive bool
	GetClient func() (rmClient, error)
}

func newRmCmd() *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "rm <path> [path...]",
		Short: "删除 115 网盘中的文件或目录",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return rmRun(&rmOpts{
				Out:       cmd.OutOrStdout(),
				Paths:     args,
				Recursive: recursive,
				GetClient: func() (rmClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "递归删除目录")
	return cmd
}

func rmRun(o *rmOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	for _, path := range o.Paths {
		entry, err := client.Stat(path)
		if err != nil {
			return fmt.Errorf("路径不存在: %s", path)
		}
		if entry.Type == "dir" && !o.Recursive {
			return fmt.Errorf("%q 是目录，需要 -r", path)
		}
	}

	if err := client.Delete(o.Paths); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}

	fmt.Fprintf(o.Out, "已删除 %d 个项目\n", len(o.Paths))
	return nil
}

func init() {
	rootCmd.AddCommand(newRmCmd())
}
