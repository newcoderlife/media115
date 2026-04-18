package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type dedupClient interface {
	ListDir(path string) ([]cloud115.Entry, error)
	ListDirUncached(path string) ([]cloud115.Entry, error)
	DeleteByIDs(fids []string, refreshDirs []string) error
	RefreshPaths(paths []string) error
	Close() error
}

type dedupOpts struct {
	Out       io.Writer
	Path      string
	Execute   bool
	GetClient func() (dedupClient, error)
}

func newDedupCmd() *cobra.Command {
	var execute bool
	cmd := &cobra.Command{
		Use:   "dedup <path>",
		Short: "清理目录下的重复文件。按文件名分组，同名文件保留一个。",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return dedupRun(&dedupOpts{
				Out:     cmd.OutOrStdout(),
				Path:    args[0],
				Execute: execute,
				GetClient: func() (dedupClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "执行删除（默认 dry-run）")
	return cmd
}

func dedupRun(o *dedupOpts) error {
	w := o.Out
	path := o.Path

	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	items, err := client.ListDir(path)
	if err != nil {
		return fmt.Errorf("列出目录失败: %w", err)
	}

	var subdirs []string
	for _, item := range items {
		if item.Type == "dir" {
			subdirs = append(subdirs, item.Name)
		}
	}

	if len(subdirs) == 0 {
		fmt.Fprintln(w, "没有子目录")
		return nil
	}

	totalDupes := 0
	var allDupeFIDs []string
	touchedDirs := map[string]struct{}{}

	for _, dirName := range subdirs {
		subdirPath := trimSlash(path) + "/" + dirName
		files, err := client.ListDirUncached(subdirPath)
		if err != nil {
			fmt.Fprintf(w, "  警告: 无法列出 %s: %v\n", subdirPath, err)
			continue
		}

		byName := map[string][]string{}
		for _, f := range files {
			if f.Type != "file" {
				continue
			}
			byName[f.Name] = append(byName[f.Name], f.FID)
		}

		type dupEntry struct{ name, fid string }
		var dupes []dupEntry
		for name, fids := range byName {
			if len(fids) > 1 {
				for _, fid := range fids[1:] {
					dupes = append(dupes, dupEntry{name, fid})
				}
			}
		}

		if len(dupes) > 0 {
			fmt.Fprintf(w, "%s/: %d 个重复文件\n", dirName, len(dupes))
			shown := map[string]bool{}
			displayed := 0
			for _, d := range dupes {
				if displayed >= 5 {
					fmt.Fprintln(w, "  ...")
					break
				}
				if !shown[d.name] {
					count := 0
					for _, dd := range dupes {
						if dd.name == d.name {
							count++
						}
					}
					fmt.Fprintf(w, "  %s (x%d 副本)\n", d.name, count)
					shown[d.name] = true
					displayed++
				}
			}
			totalDupes += len(dupes)
			for _, d := range dupes {
				if d.fid != "" {
					allDupeFIDs = append(allDupeFIDs, d.fid)
				}
			}
			touchedDirs[subdirPath] = struct{}{}
		}
	}

	fmt.Fprintf(w, "\n共 %d 个重复文件待清理\n", totalDupes)

	if len(allDupeFIDs) == 0 {
		return nil
	}

	if o.Execute {
		batchSize := 50
		deleted := 0
		for i := 0; i < len(allDupeFIDs); i += batchSize {
			end := i + batchSize
			if end > len(allDupeFIDs) {
				end = len(allDupeFIDs)
			}
			batch := allDupeFIDs[i:end]
			if err := client.DeleteByIDs(batch, nil); err != nil {
				return fmt.Errorf("删除批次失败: %w", err)
			}
			deleted += len(batch)
			fmt.Fprintf(w, "  已删除 %d/%d\n", deleted, len(allDupeFIDs))
		}
		if len(touchedDirs) > 0 {
			dirs := make([]string, 0, len(touchedDirs))
			for d := range touchedDirs {
				dirs = append(dirs, d)
			}
			if err := client.RefreshPaths(dirs); err != nil {
				fmt.Fprintf(w, "  警告: 刷新目录缓存失败: %v\n", err)
			}
		}
		fmt.Fprintf(w, "清理完成: 删除 %d 个重复文件\n", len(allDupeFIDs))
	} else {
		fmt.Fprintln(w, "(dry-run) 使用 --execute 执行删除")
	}
	return nil
}

func trimSlash(s string) string {
	for len(s) > 1 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func init() {
	rootCmd.AddCommand(newDedupCmd())
}
