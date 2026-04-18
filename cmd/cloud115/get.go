package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type getFileClient interface {
	FindFile(path string) (*cloud115.Entry, error)
	DownloadURL(pickCode, userAgent string) (string, error)
	Close() error
}

type getOpts struct {
	Out        io.Writer
	RemotePath string
	LocalDir   string
	GetClient  func() (getFileClient, error)
	Downloader func(url, dest string) error
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <remote_path> [local_dir]",
		Short: "从 115 网盘下载文件到本地目录",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localDir := "."
			if len(args) > 1 {
				localDir = args[1]
			}
			return getRun(&getOpts{
				Out:        cmd.OutOrStdout(),
				RemotePath: args[0],
				LocalDir:   localDir,
				GetClient: func() (getFileClient, error) {
					return getClient()
				},
				Downloader: downloadToFile,
			})
		},
	}
	return cmd
}

func getRun(o *getOpts) error {
	cl, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close() }()

	entry, err := cl.FindFile(o.RemotePath)
	if err != nil {
		return fmt.Errorf("远程文件不存在: %s", o.RemotePath)
	}

	if entry.PickCode == "" {
		return fmt.Errorf("文件缺少 pick_code: %s", o.RemotePath)
	}

	url, err := cl.DownloadURL(entry.PickCode, "")
	if err != nil {
		return fmt.Errorf("获取下载链接失败: %w", err)
	}

	dest := filepath.Join(o.LocalDir, entry.Name)
	if err := o.Downloader(url, dest); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	fmt.Fprintf(o.Out, "已下载: %s\n", dest)
	return nil
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
	rootCmd.AddCommand(newGetCmd())
}
