package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

var (
	lsLong      bool
	lsRecursive bool
	lsDepth     int
)

var lsCmd = &cobra.Command{
	Use:   "ls [path]",
	Short: "列出 115 网盘目录内容",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "/"
		if len(args) > 0 {
			path = args[0]
		}

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		items, err := client.ListDir(path)
		if err != nil {
			return fmt.Errorf("目录不存在: %s", path)
		}

		lsDirPrint(client, items, path, lsLong, lsRecursive, lsDepth, 0)
		return nil
	},
}

func lsDirPrint(client *cloud115.Client, items []cloud115.Entry, label string, long bool, recursive bool, depth int, indent int) {
	prefix := strings.Repeat("  ", indent)
	for _, item := range items {
		if long {
			if item.Type == "dir" {
				fmt.Printf("%sd  %8s  %s/\n", prefix, "", item.Name)
			} else {
				fmt.Printf("%sf  %8s  %s\n", prefix, formatSize(item.Size), item.Name)
			}
		} else {
			if item.Type == "dir" {
				fmt.Printf("%s%s/\n", prefix, item.Name)
			} else {
				fmt.Printf("%s%s\n", prefix, item.Name)
			}
		}

		if recursive && item.Type == "dir" && depth > 0 {
			childPath := strings.TrimRight(label, "/") + "/" + item.Name
			children, err := client.ListDir(childPath)
			if err == nil {
				lsDirPrint(client, children, childPath, long, recursive, depth-1, indent+1)
			}
		}
	}
}

func init() {
	lsCmd.Flags().BoolVarP(&lsLong, "long", "l", false, "长格式（显示大小和类型）")
	lsCmd.Flags().BoolVarP(&lsRecursive, "recursive", "R", false, "递归显示")
	lsCmd.Flags().IntVar(&lsDepth, "depth", 2, "递归深度（配合 -R，默认 2）")
	rootCmd.AddCommand(lsCmd)
}
