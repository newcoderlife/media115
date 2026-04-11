package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var findCmd = &cobra.Command{
	Use:   "find <keyword> [path]",
	Short: "在 115 网盘中搜索文件",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyword := args[0]
		path := "/"
		if len(args) > 1 {
			path = args[1]
		}

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		results, err := client.Search(keyword, path)
		if err != nil {
			return fmt.Errorf("搜索失败: %w", err)
		}

		for _, r := range results {
			fmt.Println(r.Name)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(findCmd)
}
