package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var mkdirParents bool

var mkdirCmd = &cobra.Command{
	Use:   "mkdir <path>",
	Short: "在 115 网盘中创建目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		_, err = client.Mkdir(path, mkdirParents)
		if err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}

		fmt.Printf("已创建: %s\n", path)
		return nil
	},
}

func init() {
	mkdirCmd.Flags().BoolVarP(&mkdirParents, "parents", "p", false, "递归创建中间目录")
	rootCmd.AddCommand(mkdirCmd)
}
