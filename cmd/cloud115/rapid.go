package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rapidCmd = &cobra.Command{
	Use:   "rapid <local_path> <remote_dir>",
	Short: "秒传：只传哈希，115 端去重。失败不 fallback。",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		localPath := args[0]
		remoteDir := args[1]
		filename := filepath.Base(localPath)

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		result, err := client.RapidUpload(localPath, remoteDir)
		if err != nil {
			return fmt.Errorf("秒传失败: %w", err)
		}

		if result.Status == 2 {
			fmt.Printf("秒传成功: %s (pickcode=%s)\n", filename, result.PickCode)
		} else {
			return fmt.Errorf("秒传失败: %s（115 上没有此文件）", filename)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rapidCmd)
}
