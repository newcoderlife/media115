package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type mkdirClient interface {
	Mkdir(path string, parents bool) (string, error)
	Close() error
}

type mkdirOpts struct {
	Out       io.Writer
	Path      string
	Parents   bool
	GetClient func() (mkdirClient, error)
}

func newMkdirCmd() *cobra.Command {
	var parents bool
	cmd := &cobra.Command{
		Use:   "mkdir <path>",
		Short: "在 115 网盘中创建目录",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mkdirRun(&mkdirOpts{
				Out:     cmd.OutOrStdout(),
				Path:    args[0],
				Parents: parents,
				GetClient: func() (mkdirClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVarP(&parents, "parents", "p", false, "递归创建中间目录")
	return cmd
}

func mkdirRun(o *mkdirOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, err = client.Mkdir(o.Path, o.Parents)
	if err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	fmt.Fprintf(o.Out, "已创建: %s\n", o.Path)
	return nil
}

func init() {
	rootCmd.AddCommand(newMkdirCmd())
}
