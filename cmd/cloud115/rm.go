package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rmRecursive bool

var rmCmd = &cobra.Command{
	Use:   "rm <path> [path...]",
	Short: "删除 115 网盘中的文件或目录",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		// Validate paths: check dirs require -r
		for _, path := range args {
			entry, err := client.Stat(path)
			if err != nil {
				return fmt.Errorf("路径不存在: %s", path)
			}
			if entry.Type == "dir" && !rmRecursive {
				return fmt.Errorf("%q 是目录，需要 -r", path)
			}
		}

		if err := client.Delete(args); err != nil {
			return fmt.Errorf("删除失败: %w", err)
		}

		fmt.Printf("已删除 %d 个项目\n", len(args))
		return nil
	},
}

func init() {
	rmCmd.Flags().BoolVarP(&rmRecursive, "recursive", "r", false, "递归删除目录")
	rootCmd.AddCommand(rmCmd)
}
