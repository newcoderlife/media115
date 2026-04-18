package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type renameClient interface {
	Rename(path, newName string) error
	BatchRename(renames []cloud115.BatchRenameItem) error
	Close() error
}

type renameOpts struct {
	Out       io.Writer
	Args      []string
	Batch     bool
	Stdin     io.Reader
	GetClient func() (renameClient, error)
}

func newRenameCmd() *cobra.Command {
	var batch bool
	cmd := &cobra.Command{
		Use:   "rename <path> <new_name>",
		Short: "重命名 115 网盘中的文件或目录",
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return renameRun(&renameOpts{
				Out:   cmd.OutOrStdout(),
				Args:  args,
				Batch: batch,
				Stdin: os.Stdin,
				GetClient: func() (renameClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVar(&batch, "batch", false, "从 stdin 读 JSON 批量重命名")
	return cmd
}

func renameRun(o *renameOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if o.Batch {
		raw, err := io.ReadAll(o.Stdin)
		if err != nil {
			return fmt.Errorf("读取 stdin 失败: %w", err)
		}
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
		fmt.Fprintf(o.Out, "已批量重命名 %d 个文件\n", len(items))
		return nil
	}

	if len(o.Args) != 2 {
		return fmt.Errorf("需要提供 PATH 和 NEW_NAME，或使用 --batch 模式")
	}
	path := o.Args[0]
	newName := o.Args[1]
	oldName := path[strings.LastIndex(path, "/")+1:]

	if err := client.Rename(path, newName); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	fmt.Fprintf(o.Out, "已重命名: %s → %s\n", oldName, newName)
	return nil
}

func init() {
	rootCmd.AddCommand(newRenameCmd())
}
