package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type rapidClient interface {
	RapidUpload(localPath, remoteDir string) (*cloud115.RapidResult, error)
	Close() error
}

type rapidOpts struct {
	Out       io.Writer
	LocalPath string
	RemoteDir string
	GetClient func() (rapidClient, error)
}

func newRapidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rapid <local_path> <remote_dir>",
		Short: "秒传：只传哈希，115 端去重。失败不 fallback。",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return rapidRun(&rapidOpts{
				Out:       cmd.OutOrStdout(),
				LocalPath: args[0],
				RemoteDir: args[1],
				GetClient: func() (rapidClient, error) {
					return getClient()
				},
			})
		},
	}
	return cmd
}

func rapidRun(o *rapidOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	filename := filepath.Base(o.LocalPath)
	result, err := client.RapidUpload(o.LocalPath, o.RemoteDir)
	if err != nil {
		return fmt.Errorf("秒传失败: %w", err)
	}

	if result.Status == 2 {
		fmt.Fprintf(o.Out, "秒传成功: %s (pickcode=%s)\n", filename, result.PickCode)
	} else {
		return fmt.Errorf("秒传失败: %s（115 上没有此文件）", filename)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newRapidCmd())
}
