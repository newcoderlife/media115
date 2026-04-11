package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/spf13/cobra"
)

var (
	authCheck bool
	authRenew bool
	authQR    bool
	authForce bool
	authApp   string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "115 网盘登录认证",
	Long:  "通过二维码登录 115 网盘并保存 cookies。",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

		if authCheck {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.Auth.Cookies == "" {
				fmt.Println("未登录（无 cookies）。请先运行: cloud115 auth")
				return nil
			}
			client, err := cloud115.NewClient(cfg.Auth.Cookies, cloud115.WithLogger(logger))
			if err != nil {
				return err
			}
			defer client.Close()
			if client.CheckLogin() {
				fmt.Println("已登录 115。")
			} else {
				fmt.Println("未登录。请先运行: cloud115 auth")
			}
			return nil
		}

		if authRenew {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.Auth.Cookies == "" {
				return fmt.Errorf("无已有 cookies，请先运行 cloud115 auth 登录")
			}
			client, err := cloud115.NewClient(cfg.Auth.Cookies, cloud115.WithLogger(logger))
			if err != nil {
				return err
			}
			defer client.Close()
			if !client.RenewCookies() {
				return fmt.Errorf("cookie 续期失败，请重新运行 cloud115 auth 登录")
			}
			cfg.Auth.Cookies = client.GetCookies()
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("保存 config: %w", err)
			}
			fmt.Println("Cookies 已续期并保存。")
			return nil
		}

		// Default: QR login (--qr flag or no flags)
		if !authForce {
			cfg, err := config.Load()
			if err == nil && cfg.Auth.Cookies != "" {
				existingClient, cerr := cloud115.NewClient(cfg.Auth.Cookies, cloud115.WithLogger(logger))
				if cerr == nil {
					defer existingClient.Close()
					if existingClient.CheckLogin() {
						fmt.Println("已登录 115。如需重新登录请使用 --force。")
						return nil
					}
				}
			}
		}

		// Do QR login
		tmpClient, err := cloud115.NewClient("", cloud115.WithLogger(logger))
		if err != nil {
			return fmt.Errorf("初始化 client: %w", err)
		}
		defer tmpClient.Close()

		if err := tmpClient.QRLogin(authApp); err != nil {
			return fmt.Errorf("QR 登录失败: %w", err)
		}

		cfg, err := config.Load()
		if err != nil {
			cfg = config.Default()
		}
		cfg.Auth.Cookies = tmpClient.GetCookies()
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("保存 config: %w", err)
		}
		fmt.Printf("登录成功！Cookies 已保存到 %s\n", config.ConfigPath())
		return nil
	},
}

func init() {
	authCmd.Flags().BoolVar(&authCheck, "check", false, "检查登录状态")
	authCmd.Flags().BoolVar(&authRenew, "renew", false, "自动续期 cookies")
	authCmd.Flags().BoolVar(&authQR, "qr", false, "生成二维码 URL（非阻塞）")
	authCmd.Flags().BoolVar(&authForce, "force", false, "强制重新登录（即使已登录）")
	authCmd.Flags().StringVar(&authApp, "app", "tv", "设备类型（tv/qandroid/web）")
	rootCmd.AddCommand(authCmd)
}
