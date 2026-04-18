package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type mvClient interface {
	ResolvePath(path string) (string, error)
	Move(srcPaths []string, destPath string) error
	Rename(path, newName string) error
	Close() error
}

type mvOpts struct {
	Out       io.Writer
	Args      []string
	GetClient func() (mvClient, error)
}

func newMvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mv <src> [src...] <dest>",
		Short: "移动或重命名 115 网盘中的文件",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mvRun(&mvOpts{
				Out:  cmd.OutOrStdout(),
				Args: args,
				GetClient: func() (mvClient, error) {
					return getClient()
				},
			})
		},
	}
	return cmd
}

func mvRun(o *mvOpts) error {
	srcs := o.Args[:len(o.Args)-1]
	dest := o.Args[len(o.Args)-1]

	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if len(o.Args) >= 3 {
		if _, err := client.ResolvePath(dest); err != nil {
			return fmt.Errorf("目标目录不存在: %q", dest)
		}
		if err := client.Move(srcs, dest); err != nil {
			return fmt.Errorf("移动失败: %w", err)
		}
		fmt.Fprintf(o.Out, "已移动 %d 个文件到 %s\n", len(srcs), dest)
		return nil
	}

	src := srcs[0]

	if _, err := client.ResolvePath(dest); err == nil {
		if err := client.Move([]string{src}, dest); err != nil {
			return fmt.Errorf("移动失败: %w", err)
		}
		fmt.Fprintf(o.Out, "已移动: %s → %s\n", src, dest)
		return nil
	}

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

	srcParentN := strings.TrimRight(srcParent, "/")
	if srcParentN == "" {
		srcParentN = "/"
	}
	destParentN := strings.TrimRight(destParent, "/")
	if destParentN == "" {
		destParentN = "/"
	}

	if srcParentN == destParentN {
		if err := client.Rename(src, destName); err != nil {
			return fmt.Errorf("重命名失败: %w", err)
		}
		srcName := cleanSrc[strings.LastIndex(cleanSrc, "/")+1:]
		fmt.Fprintf(o.Out, "已重命名: %s → %s\n", srcName, destName)
	} else {
		if err := client.Move([]string{src}, destParent); err != nil {
			return fmt.Errorf("移动失败: %w", err)
		}
		srcName := cleanSrc[strings.LastIndex(cleanSrc, "/")+1:]
		newPath := strings.TrimRight(destParent, "/") + "/" + srcName
		if err := client.Rename(newPath, destName); err != nil {
			return fmt.Errorf("重命名失败: %w", err)
		}
		fmt.Fprintf(o.Out, "已移动并重命名: %s → %s\n", src, dest)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newMvCmd())
}
