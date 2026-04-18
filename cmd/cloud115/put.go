package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type putClient interface {
	RapidUpload(localPath, remoteDir string) (*cloud115.RapidResult, error)
	Upload(localPath, remoteDir, filename string) (map[string]any, error)
	Close() error
}

type putOpts struct {
	Out       io.Writer
	LocalPath string
	RemoteDir string
	NoRapid   bool
	GetClient func() (putClient, error)
}

func newPutCmd() *cobra.Command {
	var noRapid bool
	cmd := &cobra.Command{
		Use:   "put <local_path> <remote_dir>",
		Short: "上传本地文件到 115 网盘目录",
		Long:  "默认先尝试秒传，失败则走普通上传。",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return putRun(&putOpts{
				Out:       cmd.OutOrStdout(),
				LocalPath: args[0],
				RemoteDir: args[1],
				NoRapid:   noRapid,
				GetClient: func() (putClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVar(&noRapid, "no-rapid", false, "跳过秒传，直接走普通上传")
	return cmd
}

func putRun(o *putOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	filename := filepath.Base(o.LocalPath)

	if !o.NoRapid {
		result, err := client.RapidUpload(o.LocalPath, o.RemoteDir)
		if err == nil && result.Status == 2 {
			fmt.Fprintf(o.Out, "已上传（秒传）: %s\n", filename)
			return nil
		}
	}

	_, err = client.Upload(o.LocalPath, o.RemoteDir, "")
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	fmt.Fprintf(o.Out, "已上传: %s\n", filename)
	return nil
}

func init() {
	rootCmd.AddCommand(newPutCmd())
}
