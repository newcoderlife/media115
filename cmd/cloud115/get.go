package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <remote_path> [local_dir]",
	Short: "从 115 网盘下载文件到本地目录",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		remotePath := args[0]
		localDir := "."
		if len(args) > 1 {
			localDir = args[1]
		}

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		entry, err := client.FindFile(remotePath)
		if err != nil {
			return fmt.Errorf("远程文件不存在: %s", remotePath)
		}

		if entry.PickCode == "" {
			return fmt.Errorf("文件缺少 pick_code: %s", remotePath)
		}

		url, err := client.DownloadURL(entry.PickCode, "")
		if err != nil {
			return fmt.Errorf("获取下载链接失败: %w", err)
		}

		dest := filepath.Join(localDir, entry.Name)
		if err := downloadToFile(url, dest); err != nil {
			return fmt.Errorf("下载失败: %w", err)
		}
		fmt.Printf("已下载: %s\n", dest)
		return nil
	},
}

func downloadToFile(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func init() {
	rootCmd.AddCommand(getCmd)
}
