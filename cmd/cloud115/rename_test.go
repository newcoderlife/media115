package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockRenameClient struct {
	renameFn      func(path, newName string) error
	batchRenameFn func(renames []cloud115.BatchRenameItem) error
}

func (m *mockRenameClient) Rename(path, newName string) error { return m.renameFn(path, newName) }
func (m *mockRenameClient) BatchRename(renames []cloud115.BatchRenameItem) error {
	return m.batchRenameFn(renames)
}
func (m *mockRenameClient) Close() error { return nil }

func TestRenameRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *renameOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &renameOpts{
				Args:      []string{"/file", "new"},
				GetClient: func() (renameClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "no args no batch",
			opts: &renameOpts{
				Args: []string{},
				GetClient: func() (renameClient, error) {
					return &mockRenameClient{}, nil
				},
			},
			wantErr: "需要提供 PATH 和 NEW_NAME",
		},
		{
			name: "single rename success",
			opts: &renameOpts{
				Args: []string{"/dir/old.txt", "new.txt"},
				GetClient: func() (renameClient, error) {
					return &mockRenameClient{
						renameFn: func(_, _ string) error { return nil },
					}, nil
				},
			},
			wantOut: "已重命名: old.txt → new.txt",
		},
		{
			name: "single rename error",
			opts: &renameOpts{
				Args: []string{"/dir/old.txt", "new.txt"},
				GetClient: func() (renameClient, error) {
					return &mockRenameClient{
						renameFn: func(_, _ string) error { return errors.New("api error") },
					}, nil
				},
			},
			wantErr: "重命名失败",
		},
		{
			name: "batch rename success",
			opts: &renameOpts{
				Args:  []string{},
				Batch: true,
				Stdin: strings.NewReader(`[["/a/b.txt","c.txt"],["/d/e.txt","f.txt"]]`),
				GetClient: func() (renameClient, error) {
					return &mockRenameClient{
						batchRenameFn: func(r []cloud115.BatchRenameItem) error { return nil },
					}, nil
				},
			},
			wantOut: "已批量重命名 2 个文件",
		},
		{
			name: "batch bad json",
			opts: &renameOpts{
				Args:  []string{},
				Batch: true,
				Stdin: strings.NewReader(`invalid`),
				GetClient: func() (renameClient, error) {
					return &mockRenameClient{}, nil
				},
			},
			wantErr: "JSON 解析失败",
		},
		{
			name: "batch rename error",
			opts: &renameOpts{
				Args:  []string{},
				Batch: true,
				Stdin: strings.NewReader(`[["/a/b.txt","c.txt"]]`),
				GetClient: func() (renameClient, error) {
					return &mockRenameClient{
						batchRenameFn: func(_ []cloud115.BatchRenameItem) error { return errors.New("api error") },
					}, nil
				},
			},
			wantErr: "批量重命名失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := renameRun(tt.opts)
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
