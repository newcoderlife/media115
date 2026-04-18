package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

type cloudDoctorClient interface {
	CheckLogin() bool
	CacheStatus() (*cloud115.CacheStats, error)
	Close() error
}

type cloudDoctorOpts struct {
	Out        io.Writer
	ConfigPath func() string
	LoadConfig func() (*config.Config, error)
	GetClient  func() (cloudDoctorClient, error)
	CacheDir   func() string
}

func newCloudDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "检查环境状态和配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cloudDoctorRun(&cloudDoctorOpts{
				Out:        cmd.OutOrStdout(),
				ConfigPath: config.ConfigPath,
				LoadConfig: config.Load,
				GetClient: func() (cloudDoctorClient, error) {
					return getClient()
				},
				CacheDir: config.CacheDir,
			})
		},
	}
}

func cloudDoctorRun(o *cloudDoctorOpts) error {
	w := o.Out
	fmt.Fprintln(w, "cloud115 环境检查:")
	fmt.Fprintln(w)

	cfgPath := o.ConfigPath()
	fmt.Fprintln(w, "配置:")
	fmt.Fprintf(w, "  配置文件:      %s\n", cfgPath)

	cfg, err := o.LoadConfig()
	if err != nil {
		fmt.Fprintf(w, "  配置加载:      ✗ 失败 (%v)\n", err)
		cfg = config.Default()
	} else {
		fmt.Fprintln(w, "  配置加载:      ✓")
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "认证:")
	if cfg.Auth.Cookies == "" {
		fmt.Fprintln(w, "  115 Cookies:   ✗ 未配置")
		if cfg.Auth.TMDB.Token != "" {
			fmt.Fprintln(w, "  TMDB Token:    ✓ 已配置")
		} else {
			fmt.Fprintln(w, "  TMDB Token:    ✗ 未配置")
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "登录状态:")
		fmt.Fprintln(w, "  115 登录:      ✗ 未配置 cookies（请运行 cloud115 auth）")
	} else {
		client, clientErr := o.GetClient()
		if clientErr != nil {
			fmt.Fprintln(w, "  115 Cookies:   ✗ 配置但无法创建客户端")
			if cfg.Auth.TMDB.Token != "" {
				fmt.Fprintln(w, "  TMDB Token:    ✓ 已配置")
			} else {
				fmt.Fprintln(w, "  TMDB Token:    ✗ 未配置")
			}
		} else {
			defer func() { _ = client.Close() }()
			loggedIn := client.CheckLogin()
			if loggedIn {
				fmt.Fprintln(w, "  115 Cookies:   ✓ 已登录")
			} else {
				fmt.Fprintln(w, "  115 Cookies:   ✗ Cookie 过期")
			}
			if cfg.Auth.TMDB.Token != "" {
				fmt.Fprintln(w, "  TMDB Token:    ✓ 已配置")
			} else {
				fmt.Fprintln(w, "  TMDB Token:    ✗ 未配置")
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, "登录状态:")
			if loggedIn {
				fmt.Fprintln(w, "  115 登录:      ✓ 有效")
			} else {
				fmt.Fprintln(w, "  115 登录:      ✗ cookie 已过期（请运行 cloud115 auth）")
			}

			fmt.Fprintln(w)
			fmt.Fprintln(w, "缓存:")
			fmt.Fprintf(w, "  缓存目录:      %s\n", o.CacheDir())
			cstats, err := client.CacheStatus()
			if err != nil {
				fmt.Fprintf(w, "  缓存状态:      ✗ 获取失败 (%v)\n", err)
			} else {
				fmt.Fprintf(w, "  路径映射:      %d 条目\n", cstats.PathCount)
				fmt.Fprintf(w, "  目录缓存:      %d 个目录, %d 条目\n", cstats.DirCount, cstats.EntryCount)
				fmt.Fprintf(w, "  目录树:        %d 条目\n", cstats.TreeEntries)
				fmt.Fprintf(w, "  数据库大小:    %s\n", formatSize(cstats.DBSizeBytes))

				now := float64(time.Now().Unix())
				for name, state := range cstats.RateLimit {
					if now < state.CooldownUntil {
						remaining := int(state.CooldownUntil - now)
						fmt.Fprintf(w, "  限流 (%s):    ⚠ cooldown 中 (剩余 %ds)\n", name, remaining)
					} else {
						fmt.Fprintf(w, "  限流 (%s):    正常 (%d/20 QPM)\n", name, state.MinuteCount)
					}
				}
			}
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "代理配置:")
	fmt.Fprintf(w, "  strm-proxy:    %s:%d\n", cfg.Proxy.Host, cfg.Proxy.Port)
	fmt.Fprintf(w, "  Jellyfin URL:  %s\n", cfg.Proxy.JellyfinURL)

	return nil
}

func init() {
	rootCmd.AddCommand(newCloudDoctorCmd())
}
