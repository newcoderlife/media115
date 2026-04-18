package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type mockMvClient struct {
	resolveFn func(path string) (string, error)
	moveFn    func(srcs []string, dest string) error
	renameFn  func(path, newName string) error
}

func (m *mockMvClient) ResolvePath(path string) (string, error) { return m.resolveFn(path) }
func (m *mockMvClient) Move(srcs []string, dest string) error   { return m.moveFn(srcs, dest) }
func (m *mockMvClient) Rename(path, name string) error          { return m.renameFn(path, name) }
func (m *mockMvClient) Close() error                            { return nil }

func TestMvRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *mvOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &mvOpts{
				Args:      []string{"/a", "/b"},
				GetClient: func() (mvClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "multi src dest not exist",
			opts: &mvOpts{
				Args: []string{"/a", "/b", "/dest"},
				GetClient: func() (mvClient, error) {
					return &mockMvClient{
						resolveFn: func(_ string) (string, error) { return "", errors.New("not found") },
					}, nil
				},
			},
			wantErr: "目标目录不存在",
		},
		{
			name: "multi src move error",
			opts: &mvOpts{
				Args: []string{"/a", "/b", "/dest"},
				GetClient: func() (mvClient, error) {
					return &mockMvClient{
						resolveFn: func(_ string) (string, error) { return "123", nil },
						moveFn:    func(_ []string, _ string) error { return errors.New("api err") },
					}, nil
				},
			},
			wantErr: "移动失败",
		},
		{
			name: "multi src success",
			opts: &mvOpts{
				Args: []string{"/a", "/b", "/dest"},
				GetClient: func() (mvClient, error) {
					return &mockMvClient{
						resolveFn: func(_ string) (string, error) { return "123", nil },
						moveFn:    func(_ []string, _ string) error { return nil },
					}, nil
				},
			},
			wantOut: "已移动 2 个文件到 /dest",
		},
		{
			name: "dest is existing dir",
			opts: &mvOpts{
				Args: []string{"/src/file.txt", "/dest"},
				GetClient: func() (mvClient, error) {
					return &mockMvClient{
						resolveFn: func(_ string) (string, error) { return "123", nil },
						moveFn:    func(_ []string, _ string) error { return nil },
					}, nil
				},
			},
			wantOut: "已移动: /src/file.txt → /dest",
		},
		{
			name: "dest existing dir move error",
			opts: &mvOpts{
				Args: []string{"/src/file.txt", "/dest"},
				GetClient: func() (mvClient, error) {
					return &mockMvClient{
						resolveFn: func(_ string) (string, error) { return "123", nil },
						moveFn:    func(_ []string, _ string) error { return errors.New("api err") },
					}, nil
				},
			},
			wantErr: "移动失败",
		},
		{
			name: "same dir rename",
			opts: &mvOpts{
				Args: []string{"/dir/old.txt", "/dir/new.txt"},
				GetClient: func() (mvClient, error) {
					callCount := 0
					return &mockMvClient{
						resolveFn: func(path string) (string, error) {
							callCount++
							if callCount == 1 {
								return "", errors.New("not found") // dest doesn't exist as dir
							}
							return "123", nil // parent exists
						},
						renameFn: func(_, _ string) error { return nil },
					}, nil
				},
			},
			wantOut: "已重命名: old.txt → new.txt",
		},
		{
			name: "same dir rename error",
			opts: &mvOpts{
				Args: []string{"/dir/old.txt", "/dir/new.txt"},
				GetClient: func() (mvClient, error) {
					callCount := 0
					return &mockMvClient{
						resolveFn: func(path string) (string, error) {
							callCount++
							if callCount == 1 {
								return "", errors.New("not found")
							}
							return "123", nil
						},
						renameFn: func(_, _ string) error { return errors.New("api err") },
					}, nil
				},
			},
			wantErr: "重命名失败",
		},
		{
			name: "cross dir move and rename",
			opts: &mvOpts{
				Args: []string{"/src/file.txt", "/dest/newname.txt"},
				GetClient: func() (mvClient, error) {
					callCount := 0
					return &mockMvClient{
						resolveFn: func(path string) (string, error) {
							callCount++
							if callCount == 1 {
								return "", errors.New("not found") // dest as dir doesn't exist
							}
							return "123", nil // parent /dest exists
						},
						moveFn:   func(_ []string, _ string) error { return nil },
						renameFn: func(_, _ string) error { return nil },
					}, nil
				},
			},
			wantOut: "已移动并重命名",
		},
		{
			name: "cross dir move error",
			opts: &mvOpts{
				Args: []string{"/src/file.txt", "/dest/newname.txt"},
				GetClient: func() (mvClient, error) {
					callCount := 0
					return &mockMvClient{
						resolveFn: func(path string) (string, error) {
							callCount++
							if callCount == 1 {
								return "", errors.New("not found")
							}
							return "123", nil
						},
						moveFn: func(_ []string, _ string) error { return errors.New("api err") },
					}, nil
				},
			},
			wantErr: "移动失败",
		},
		{
			name: "cross dir rename after move error",
			opts: &mvOpts{
				Args: []string{"/src/file.txt", "/dest/newname.txt"},
				GetClient: func() (mvClient, error) {
					callCount := 0
					return &mockMvClient{
						resolveFn: func(path string) (string, error) {
							callCount++
							if callCount == 1 {
								return "", errors.New("not found")
							}
							return "123", nil
						},
						moveFn:   func(_ []string, _ string) error { return nil },
						renameFn: func(_, _ string) error { return errors.New("api err") },
					}, nil
				},
			},
			wantErr: "重命名失败",
		},
		{
			name: "dest parent not exist",
			opts: &mvOpts{
				Args: []string{"/src/file.txt", "/missing/newname.txt"},
				GetClient: func() (mvClient, error) {
					return &mockMvClient{
						resolveFn: func(_ string) (string, error) { return "", errors.New("not found") },
					}, nil
				},
			},
			wantErr: "目标父目录不存在",
		},
		{
			name: "dest no slash rename in root",
			opts: &mvOpts{
				Args: []string{"file.txt", "newname.txt"},
				GetClient: func() (mvClient, error) {
					callCount := 0
					return &mockMvClient{
						resolveFn: func(path string) (string, error) {
							callCount++
							if callCount == 1 {
								return "", errors.New("not found") // dest not dir
							}
							return "123", nil // root exists
						},
						renameFn: func(_, _ string) error { return nil },
					}, nil
				},
			},
			wantOut: "已重命名",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := mvRun(tt.opts)
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
