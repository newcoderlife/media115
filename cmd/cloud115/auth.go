package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/logging"
)

type authClient interface {
	CheckLogin() bool
	RenewCookies() bool
	GetCookies() string
	QRGetToken(app string) (*cloud115.QRSession, error)
	QRWaitAndLogin(sess *cloud115.QRSession) error
	QRLogin(app string) error
	Close() error
}

type authOpts struct {
	Out        io.Writer
	Check      bool
	Renew      bool
	Force      bool
	App        string
	GetQR      bool
	WaitQR     bool
	NewClient  func(cookies string) (authClient, error)
	LoadCfg    func() (*config.Config, error)
	DefaultCfg func() *config.Config
	CfgPath    func() string
	TempDir    string
	WriteFile  func(name string, data []byte, perm os.FileMode) error
	ReadFile   func(name string) ([]byte, error)
	RemoveFile func(name string) error
	SaveConfig func(*config.Config) error
}

func newAuthCmd() *cobra.Command {
	var (
		check  bool
		renew  bool
		force  bool
		app    string
		getQR  bool
		waitQR bool
	)
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "115 网盘登录认证",
		Long:  "通过二维码登录 115 网盘并保存 cookies。",
		RunE: func(cmd *cobra.Command, args []string) error {
			logging.Setup(verbose)
			return authRun(&authOpts{
				Out:    cmd.OutOrStdout(),
				Check:  check,
				Renew:  renew,
				Force:  force,
				App:    app,
				GetQR:  getQR,
				WaitQR: waitQR,
				NewClient: func(cookies string) (authClient, error) {
					return cloud115.NewClient(cookies)
				},
				LoadCfg:    config.Load,
				DefaultCfg: config.Default,
				CfgPath:    config.ConfigPath,
				TempDir:    os.TempDir(),
				WriteFile:  os.WriteFile,
				ReadFile:   os.ReadFile,
				RemoveFile: os.Remove,
				SaveConfig: func(cfg *config.Config) error { return cfg.Save() },
			})
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "检查登录状态")
	cmd.Flags().BoolVar(&renew, "renew", false, "自动续期 cookies")
	cmd.Flags().BoolVar(&force, "force", false, "强制重新登录（即使已登录）")
	cmd.Flags().StringVar(&app, "app", "tv", "设备类型（tv/qandroid/web）")
	cmd.Flags().BoolVar(&getQR, "get-qr", false, "生成 QR URL 并退出（非阻塞，第一阶段）")
	cmd.Flags().BoolVar(&waitQR, "wait-qr", false, "等待 QR 扫描完成并保存 cookies（第二阶段）")
	return cmd
}

func authRun(o *authOpts) error {
	if o.Check {
		cfg, err := o.LoadCfg()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Auth.Cookies == "" {
			fmt.Fprintln(o.Out, "未登录（无 cookies）。请先运行: cloud115 auth")
			return nil
		}
		client, err := o.NewClient(cfg.Auth.Cookies)
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()
		if client.CheckLogin() {
			fmt.Fprintln(o.Out, "已登录 115。")
		} else {
			fmt.Fprintln(o.Out, "未登录。请先运行: cloud115 auth")
		}
		return nil
	}

	if o.Renew {
		cfg, err := o.LoadCfg()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Auth.Cookies == "" {
			return fmt.Errorf("无已有 cookies，请先运行 cloud115 auth 登录")
		}
		client, err := o.NewClient(cfg.Auth.Cookies)
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()
		if !client.RenewCookies() {
			return fmt.Errorf("cookie 续期失败，请重新运行 cloud115 auth 登录")
		}
		cfg.Auth.Cookies = client.GetCookies()
		if err := o.SaveConfig(cfg); err != nil {
			return fmt.Errorf("保存 config: %w", err)
		}
		fmt.Fprintln(o.Out, "Cookies 已续期并保存。")
		return nil
	}

	if o.GetQR {
		tmpClient, err := o.NewClient("")
		if err != nil {
			return fmt.Errorf("初始化 client: %w", err)
		}
		defer func() { _ = tmpClient.Close() }()
		sess, err := tmpClient.QRGetToken(o.App)
		if err != nil {
			return fmt.Errorf("获取 QR token 失败: %w", err)
		}
		fmt.Fprintf(o.Out, "QR URL: %s\n", sess.QRURL)
		sessFile := filepath.Join(o.TempDir, "cloud115_qr_session.json")
		data, _ := json.MarshalIndent(sess, "", "  ")
		if err := o.WriteFile(sessFile, data, 0o600); err != nil {
			return fmt.Errorf("保存 QR session 失败: %w", err)
		}
		fmt.Fprintf(o.Out, "Session saved to %s\n", sessFile)
		fmt.Fprintln(o.Out, "Scan the QR code, then run: cloud115 auth --wait-qr")
		return nil
	}

	if o.WaitQR {
		sessFile := filepath.Join(o.TempDir, "cloud115_qr_session.json")
		data, err := o.ReadFile(sessFile)
		if err != nil {
			return fmt.Errorf("未找到 QR session（先运行 cloud115 auth --get-qr）: %w", err)
		}
		var sess cloud115.QRSession
		if err := json.Unmarshal(data, &sess); err != nil {
			return fmt.Errorf("解析 QR session 失败: %w", err)
		}
		tmpClient, err := o.NewClient("")
		if err != nil {
			return fmt.Errorf("初始化 client: %w", err)
		}
		defer func() { _ = tmpClient.Close() }()
		if err := tmpClient.QRWaitAndLogin(&sess); err != nil {
			return fmt.Errorf("QR 登录失败: %w", err)
		}
		_ = o.RemoveFile(sessFile)
		cfg, err := o.LoadCfg()
		if err != nil {
			cfg = o.DefaultCfg()
		}
		cfg.Auth.Cookies = tmpClient.GetCookies()
		if err := o.SaveConfig(cfg); err != nil {
			return fmt.Errorf("保存 config: %w", err)
		}
		fmt.Fprintf(o.Out, "登录成功！Cookies 已保存到 %s\n", o.CfgPath())
		return nil
	}

	// Default: QR login (no flags)
	if !o.Force {
		cfg, err := o.LoadCfg()
		if err == nil && cfg.Auth.Cookies != "" {
			existingClient, cerr := o.NewClient(cfg.Auth.Cookies)
			if cerr == nil {
				defer func() { _ = existingClient.Close() }()
				if existingClient.CheckLogin() {
					fmt.Fprintln(o.Out, "已登录 115。如需重新登录请使用 --force。")
					return nil
				}
			}
		}
	}

	tmpClient, err := o.NewClient("")
	if err != nil {
		return fmt.Errorf("初始化 client: %w", err)
	}
	defer func() { _ = tmpClient.Close() }()

	if err := tmpClient.QRLogin(o.App); err != nil {
		return fmt.Errorf("QR 登录失败: %w", err)
	}

	cfg, err := o.LoadCfg()
	if err != nil {
		cfg = o.DefaultCfg()
	}
	cfg.Auth.Cookies = tmpClient.GetCookies()
	if err := o.SaveConfig(cfg); err != nil {
		return fmt.Errorf("保存 config: %w", err)
	}
	fmt.Fprintf(o.Out, "登录成功！Cookies 已保存到 %s\n", o.CfgPath())
	return nil
}

func init() {
	rootCmd.AddCommand(newAuthCmd())
}
