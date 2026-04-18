package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type lsClient interface {
	ListDir(path string) ([]cloud115.Entry, error)
	Close() error
}

type lsOpts struct {
	Out       io.Writer
	Path      string
	Long      bool
	Recursive bool
	Depth     int
	GetClient func() (lsClient, error)
}

func newLsCmd() *cobra.Command {
	var (
		long      bool
		recursive bool
		depth     int
	)
	cmd := &cobra.Command{
		Use:   "ls [path]",
		Short: "列出 115 网盘目录内容",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/"
			if len(args) > 0 {
				path = args[0]
			}
			return lsRun(&lsOpts{
				Out:       cmd.OutOrStdout(),
				Path:      path,
				Long:      long,
				Recursive: recursive,
				Depth:     depth,
				GetClient: func() (lsClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVarP(&long, "long", "l", false, "长格式（显示大小和类型）")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "递归显示")
	cmd.Flags().IntVar(&depth, "depth", 2, "递归深度（配合 -R，默认 2）")
	return cmd
}

func lsRun(o *lsOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	items, err := client.ListDir(o.Path)
	if err != nil {
		return fmt.Errorf("目录不存在: %s", o.Path)
	}

	lsDirPrint(o.Out, client, items, o.Path, o.Long, o.Recursive, o.Depth, 0)
	return nil
}

func lsDirPrint(w io.Writer, client lsClient, items []cloud115.Entry, label string, long bool, recursive bool, depth int, indent int) {
	prefix := strings.Repeat("  ", indent)
	for _, item := range items {
		if long {
			if item.Type == "dir" {
				fmt.Fprintf(w, "%sd  %8s  %s/\n", prefix, "", item.Name)
			} else {
				fmt.Fprintf(w, "%sf  %8s  %s\n", prefix, formatSize(item.Size), item.Name)
			}
		} else {
			if item.Type == "dir" {
				fmt.Fprintf(w, "%s%s/\n", prefix, item.Name)
			} else {
				fmt.Fprintf(w, "%s%s\n", prefix, item.Name)
			}
		}

		if recursive && item.Type == "dir" && depth > 0 {
			childPath := strings.TrimRight(label, "/") + "/" + item.Name
			children, err := client.ListDir(childPath)
			if err == nil {
				lsDirPrint(w, client, children, childPath, long, recursive, depth-1, indent+1)
			}
		}
	}
}

func init() {
	rootCmd.AddCommand(newLsCmd())
}
