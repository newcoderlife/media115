package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/spf13/cobra"
)

var renameBatch bool

var renameCmd = &cobra.Command{
	Use:   "rename <path> <new_name>",
	Short: "重命名 115 网盘中的文件或目录",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		if renameBatch {
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("读取 stdin 失败: %w", err)
			}
			// Expect JSON: [[path, new_name], ...]
			var pairs [][2]string
			if err := json.Unmarshal(raw, &pairs); err != nil {
				return fmt.Errorf("JSON 解析失败: %w", err)
			}
			items := make([]cloud115.BatchRenameItem, 0, len(pairs))
			for _, p := range pairs {
				items = append(items, cloud115.BatchRenameItem{Path: p[0], NewName: p[1]})
			}
			if err := client.BatchRename(items); err != nil {
				return fmt.Errorf("批量重命名失败: %w", err)
			}
			fmt.Printf("已批量重命名 %d 个文件\n", len(items))
			return nil
		}

		if len(args) != 2 {
			return fmt.Errorf("需要提供 PATH 和 NEW_NAME，或使用 --batch 模式")
		}
		path := args[0]
		newName := args[1]
		oldName := path[strings.LastIndex(path, "/")+1:]

		if err := client.Rename(path, newName); err != nil {
			return fmt.Errorf("重命名失败: %w", err)
		}
		fmt.Printf("已重命名: %s → %s\n", oldName, newName)
		return nil
	},
}

func init() {
	renameCmd.Flags().BoolVar(&renameBatch, "batch", false, "从 stdin 读 JSON 批量重命名")
	rootCmd.AddCommand(renameCmd)
}
