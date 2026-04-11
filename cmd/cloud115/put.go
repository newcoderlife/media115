package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var putNoRapid bool

var putCmd = &cobra.Command{
	Use:   "put <local_path> <remote_dir>",
	Short: "上传本地文件到 115 网盘目录",
	Long:  "默认先尝试秒传，失败则走普通上传。",
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

		if !putNoRapid {
			result, err := client.RapidUpload(localPath, remoteDir)
			if err == nil && result.Status == 2 {
				fmt.Printf("已上传（秒传）: %s\n", filename)
				return nil
			}
		}

		_, err = client.Upload(localPath, remoteDir, "")
		if err != nil {
			return fmt.Errorf("上传失败: %w", err)
		}
		fmt.Printf("已上传: %s\n", filename)
		return nil
	},
}

func init() {
	putCmd.Flags().BoolVar(&putNoRapid, "no-rapid", false, "跳过秒传，直接走普通上传")
	rootCmd.AddCommand(putCmd)
}
