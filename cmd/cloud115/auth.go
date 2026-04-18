package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/logging"
)

var (
	authCheck  bool
	authRenew  bool
	authForce  bool
	authApp    string
	authGetQR  bool
	authWaitQR bool
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "115 网盘登录认证",
	Long:  "通过二维码登录 115 网盘并保存 cookies。",
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.Setup(verbose)

		if authCheck {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.Auth.Cookies == "" {
				fmt.Println("未登录（无 cookies）。请先运行: cloud115 auth")
				return nil
			}
			client, err := cloud115.NewClient(cfg.Auth.Cookies)
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
			client, err := cloud115.NewClient(cfg.Auth.Cookies)
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

		// --get-qr: generate QR token, print URL, save session, exit.
		if authGetQR {
			tmpClient, err := cloud115.NewClient("")
			if err != nil {
				return fmt.Errorf("初始化 client: %w", err)
			}
			defer tmpClient.Close()
			sess, err := tmpClient.QRGetToken(authApp)
			if err != nil {
				return fmt.Errorf("获取 QR token 失败: %w", err)
			}
			fmt.Printf("QR URL: %s\n", sess.QRURL)
			// Save session to temp file for --wait-qr.
			sessFile := filepath.Join(os.TempDir(), "cloud115_qr_session.json")
			data, _ := json.MarshalIndent(sess, "", "  ")
			if err := os.WriteFile(sessFile, data, 0o600); err != nil {
				return fmt.Errorf("保存 QR session 失败: %w", err)
			}
			fmt.Printf("Session saved to %s\n", sessFile)
			fmt.Println("Scan the QR code, then run: cloud115 auth --wait-qr")
			return nil
		}

		// --wait-qr: read saved session, poll for scan, save cookies.
		if authWaitQR {
			sessFile := filepath.Join(os.TempDir(), "cloud115_qr_session.json")
			data, err := os.ReadFile(sessFile)
			if err != nil {
				return fmt.Errorf("未找到 QR session（先运行 cloud115 auth --get-qr）: %w", err)
			}
			var sess cloud115.QRSession
			if err := json.Unmarshal(data, &sess); err != nil {
				return fmt.Errorf("解析 QR session 失败: %w", err)
			}
			tmpClient, err := cloud115.NewClient("")
			if err != nil {
				return fmt.Errorf("初始化 client: %w", err)
			}
			defer tmpClient.Close()
			if err := tmpClient.QRWaitAndLogin(&sess); err != nil {
				return fmt.Errorf("QR 登录失败: %w", err)
			}
			_ = os.Remove(sessFile)
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
		}

		// Default: QR login (no flags)
		if !authForce {
			cfg, err := config.Load()
			if err == nil && cfg.Auth.Cookies != "" {
				existingClient, cerr := cloud115.NewClient(cfg.Auth.Cookies)
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
		tmpClient, err := cloud115.NewClient("")
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
	authCmd.Flags().BoolVar(&authForce, "force", false, "强制重新登录（即使已登录）")
	authCmd.Flags().StringVar(&authApp, "app", "tv", "设备类型（tv/qandroid/web）")
	authCmd.Flags().BoolVar(&authGetQR, "get-qr", false, "生成 QR URL 并退出（非阻塞，第一阶段）")
	authCmd.Flags().BoolVar(&authWaitQR, "wait-qr", false, "等待 QR 扫描完成并保存 cookies（第二阶段）")
	rootCmd.AddCommand(authCmd)
}
