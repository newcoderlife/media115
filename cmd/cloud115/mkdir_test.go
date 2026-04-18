package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type mockMkdirClient struct {
	mkdirFn func(path string, parents bool) (string, error)
}

func (m *mockMkdirClient) Mkdir(path string, parents bool) (string, error) {
	return m.mkdirFn(path, parents)
}
func (m *mockMkdirClient) Close() error { return nil }

func TestMkdirRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *mkdirOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &mkdirOpts{
				Path:      "/new",
				GetClient: func() (mkdirClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "mkdir fails",
			opts: &mkdirOpts{
				Path: "/new/dir",
				GetClient: func() (mkdirClient, error) {
					return &mockMkdirClient{mkdirFn: func(_ string, _ bool) (string, error) {
						return "", errors.New("permission denied")
					}}, nil
				},
			},
			wantErr: "创建目录失败",
		},
		{
			name: "success",
			opts: &mkdirOpts{
				Path:    "/new/dir",
				Parents: true,
				GetClient: func() (mkdirClient, error) {
					return &mockMkdirClient{mkdirFn: func(path string, parents bool) (string, error) {
						if !parents {
							return "", errors.New("expected parents=true")
						}
						return "12345", nil
					}}, nil
				},
			},
			wantOut: "已创建: /new/dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := mkdirRun(tt.opts)
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
