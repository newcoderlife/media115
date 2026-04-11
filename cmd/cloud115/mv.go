package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:   "mv <src> [src...] <dest>",
	Short: "移动或重命名 115 网盘中的文件",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		srcs := args[:len(args)-1]
		dest := args[len(args)-1]

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		if len(args) >= 3 {
			// Multiple sources → dest must be an existing directory
			if _, err := client.ResolvePath(dest); err != nil {
				return fmt.Errorf("目标目录不存在: %q", dest)
			}
			if err := client.Move(srcs, dest); err != nil {
				return fmt.Errorf("移动失败: %w", err)
			}
			fmt.Printf("已移动 %d 个文件到 %s\n", len(srcs), dest)
			return nil
		}

		// Exactly 2 args: src dest
		src := srcs[0]

		// Try dest as existing directory
		if _, err := client.ResolvePath(dest); err == nil {
			if err := client.Move([]string{src}, dest); err != nil {
				return fmt.Errorf("移动失败: %w", err)
			}
			fmt.Printf("已移动: %s → %s\n", src, dest)
			return nil
		}

		// dest doesn't exist → parse parent + name using LastIndex
		cleanDest := strings.TrimRight(dest, "/")
		idx := strings.LastIndex(cleanDest, "/")
		var destParent, destName string
		if idx < 0 {
			destParent = "/"
			destName = cleanDest
		} else {
			destParent = cleanDest[:idx]
			destName = cleanDest[idx+1:]
		}
		if destParent == "" {
			destParent = "/"
		}

		if _, err := client.ResolvePath(destParent); err != nil {
			return fmt.Errorf("目标父目录不存在: %q", destParent)
		}

		cleanSrc := strings.TrimRight(src, "/")
		srcIdx := strings.LastIndex(cleanSrc, "/")
		srcParent := "/"
		if srcIdx >= 0 {
			srcParent = cleanSrc[:srcIdx]
			if srcParent == "" {
				srcParent = "/"
			}
		}

		// Normalize for comparison
		srcParentN := strings.TrimRight(srcParent, "/")
		if srcParentN == "" {
			srcParentN = "/"
		}
		destParentN := strings.TrimRight(destParent, "/")
		if destParentN == "" {
			destParentN = "/"
		}

		if srcParentN == destParentN {
			// Same directory → rename only
			if err := client.Rename(src, destName); err != nil {
				return fmt.Errorf("重命名失败: %w", err)
			}
			srcName := cleanSrc[strings.LastIndex(cleanSrc, "/")+1:]
			fmt.Printf("已重命名: %s → %s\n", srcName, destName)
		} else {
			// Cross-directory + rename: move then rename
			if err := client.Move([]string{src}, destParent); err != nil {
				return fmt.Errorf("移动失败: %w", err)
			}
			srcName := cleanSrc[strings.LastIndex(cleanSrc, "/")+1:]
			newPath := strings.TrimRight(destParent, "/") + "/" + srcName
			if err := client.Rename(newPath, destName); err != nil {
				return fmt.Errorf("重命名失败: %w", err)
			}
			fmt.Printf("已移动并重命名: %s → %s\n", src, dest)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(mvCmd)
}
