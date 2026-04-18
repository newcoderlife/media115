package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

type mockAuthClient struct {
	checkLogin bool
	renewOK    bool
	cookies    string
	qrTokenFn  func(app string) (*cloud115.QRSession, error)
	qrWaitFn   func(sess *cloud115.QRSession) error
	qrLoginFn  func(app string) error
}

func (m *mockAuthClient) CheckLogin() bool                                   { return m.checkLogin }
func (m *mockAuthClient) RenewCookies() bool                                 { return m.renewOK }
func (m *mockAuthClient) GetCookies() string                                 { return m.cookies }
func (m *mockAuthClient) QRGetToken(app string) (*cloud115.QRSession, error) { return m.qrTokenFn(app) }
func (m *mockAuthClient) QRWaitAndLogin(sess *cloud115.QRSession) error      { return m.qrWaitFn(sess) }
func (m *mockAuthClient) QRLogin(app string) error                           { return m.qrLoginFn(app) }
func (m *mockAuthClient) Close() error                                       { return nil }

func newMockCfg(cookies string) *config.Config {
	cfg := config.Default()
	cfg.Auth.Cookies = cookies
	return cfg
}

func TestAuthRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *authOpts
		wantErr string
		wantOut string
	}{
		// --check mode
		{
			name: "check config error",
			opts: &authOpts{
				Check:   true,
				LoadCfg: func() (*config.Config, error) { return nil, errors.New("no config") },
			},
			wantErr: "load config",
		},
		{
			name: "check no cookies",
			opts: &authOpts{
				Check:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg(""), nil },
			},
			wantOut: "未登录",
		},
		{
			name: "check logged in",
			opts: &authOpts{
				Check:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{checkLogin: true}, nil
				},
			},
			wantOut: "已登录",
		},
		{
			name: "check not logged in",
			opts: &authOpts{
				Check:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{checkLogin: false}, nil
				},
			},
			wantOut: "未登录",
		},
		{
			name: "check client error",
			opts: &authOpts{
				Check:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(_ string) (authClient, error) {
					return nil, errors.New("client err")
				},
			},
			wantErr: "client err",
		},

		// --renew mode
		{
			name: "renew config error",
			opts: &authOpts{
				Renew:   true,
				LoadCfg: func() (*config.Config, error) { return nil, errors.New("no config") },
			},
			wantErr: "load config",
		},
		{
			name: "renew no cookies",
			opts: &authOpts{
				Renew:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg(""), nil },
			},
			wantErr: "无已有 cookies",
		},
		{
			name: "renew client error",
			opts: &authOpts{
				Renew:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(_ string) (authClient, error) {
					return nil, errors.New("client err")
				},
			},
			wantErr: "client err",
		},
		{
			name: "renew fail",
			opts: &authOpts{
				Renew:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{renewOK: false}, nil
				},
			},
			wantErr: "cookie 续期失败",
		},
		{
			name: "renew success",
			opts: &authOpts{
				Renew:   true,
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{renewOK: true, cookies: "renewed"}, nil
				},
				SaveConfig: func(_ *config.Config) error { return nil },
			},
			wantOut: "\u5df2\u7eed\u671f",
		},
		// --get-qr mode
		{
			name: "get-qr client error",
			opts: &authOpts{
				GetQR: true,
				App:   "tv",
				NewClient: func(_ string) (authClient, error) {
					return nil, errors.New("client err")
				},
			},
			wantErr: "初始化 client",
		},
		{
			name: "get-qr token error",
			opts: &authOpts{
				GetQR: true,
				App:   "tv",
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrTokenFn: func(_ string) (*cloud115.QRSession, error) {
							return nil, errors.New("token err")
						},
					}, nil
				},
			},
			wantErr: "获取 QR token 失败",
		},
		{
			name: "get-qr success",
			opts: &authOpts{
				GetQR:   true,
				App:     "tv",
				TempDir: t.TempDir(),
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrTokenFn: func(_ string) (*cloud115.QRSession, error) {
							return &cloud115.QRSession{QRURL: "http://qr.example.com"}, nil
						},
					}, nil
				},
				WriteFile: func(_ string, _ []byte, _ os.FileMode) error { return nil },
			},
			wantOut: "QR URL:",
		},
		{
			name: "get-qr write error",
			opts: &authOpts{
				GetQR:   true,
				App:     "tv",
				TempDir: t.TempDir(),
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrTokenFn: func(_ string) (*cloud115.QRSession, error) {
							return &cloud115.QRSession{QRURL: "http://qr.example.com"}, nil
						},
					}, nil
				},
				WriteFile: func(_ string, _ []byte, _ os.FileMode) error { return errors.New("perm") },
			},
			wantErr: "保存 QR session 失败",
		},

		// --wait-qr mode
		{
			name: "wait-qr no session file",
			opts: &authOpts{
				WaitQR:   true,
				TempDir:  t.TempDir(),
				ReadFile: func(_ string) ([]byte, error) { return nil, errors.New("not found") },
			},
			wantErr: "未找到 QR session",
		},
		{
			name: "wait-qr bad json",
			opts: &authOpts{
				WaitQR:   true,
				TempDir:  t.TempDir(),
				ReadFile: func(_ string) ([]byte, error) { return []byte("invalid"), nil },
			},
			wantErr: "解析 QR session 失败",
		},
		{
			name: "wait-qr client error",
			opts: &authOpts{
				WaitQR:  true,
				TempDir: t.TempDir(),
				ReadFile: func(_ string) ([]byte, error) {
					return []byte(`{"uid":"1","time":"2","sign":"3"}`), nil
				},
				NewClient: func(_ string) (authClient, error) {
					return nil, errors.New("client err")
				},
			},
			wantErr: "初始化 client",
		},
		{
			name: "wait-qr login fail",
			opts: &authOpts{
				WaitQR:  true,
				TempDir: t.TempDir(),
				ReadFile: func(_ string) ([]byte, error) {
					return []byte(`{"uid":"1","time":"2","sign":"3"}`), nil
				},
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrWaitFn: func(_ *cloud115.QRSession) error { return errors.New("timeout") },
					}, nil
				},
			},
			wantErr: "QR 登录失败",
		},
		{
			name: "wait-qr success",
			opts: &authOpts{
				WaitQR:  true,
				TempDir: t.TempDir(),
				ReadFile: func(_ string) ([]byte, error) {
					return []byte(`{"uid":"1","time":"2","sign":"3"}`), nil
				},
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrWaitFn: func(_ *cloud115.QRSession) error { return nil },
						cookies:  "new_cookies",
					}, nil
				},
				RemoveFile: func(_ string) error { return nil },
				LoadCfg:    func() (*config.Config, error) { return newMockCfg(""), nil },
				CfgPath:    func() string { return "/tmp/config.toml" },
				SaveConfig: func(_ *config.Config) error { return nil },
			},
			wantOut: "登录成功",
		},

		// default mode (QR login)
		{
			name: "default already logged in",
			opts: &authOpts{
				LoadCfg: func() (*config.Config, error) { return newMockCfg("abc"), nil },
				NewClient: func(c string) (authClient, error) {
					if c == "abc" {
						return &mockAuthClient{checkLogin: true}, nil
					}
					return &mockAuthClient{}, nil
				},
			},
			wantOut: "已登录 115",
		},
		{
			name: "default force relogin",
			opts: &authOpts{
				Force: true,
				App:   "tv",
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrLoginFn: func(_ string) error { return nil },
						cookies:   "new",
					}, nil
				},
				LoadCfg:    func() (*config.Config, error) { return newMockCfg(""), nil },
				DefaultCfg: config.Default,
				CfgPath:    func() string { return "/tmp/config.toml" },
				SaveConfig: func(_ *config.Config) error { return nil },
			},
			wantOut: "登录成功",
		},
		{
			name: "default qr login error",
			opts: &authOpts{
				Force: true,
				App:   "tv",
				NewClient: func(_ string) (authClient, error) {
					return &mockAuthClient{
						qrLoginFn: func(_ string) error { return errors.New("cancel") },
					}, nil
				},
			},
			wantErr: "QR 登录失败",
		},
		{
			name: "default client init error",
			opts: &authOpts{
				Force: true,
				NewClient: func(_ string) (authClient, error) {
					return nil, errors.New("init err")
				},
			},
			wantErr: "初始化 client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := authRun(tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantOut != "" && !strings.Contains(buf.String(), tt.wantOut) {
				t.Errorf("expected output containing %q, got: %q", tt.wantOut, buf.String())
			}
		})
	}
}
