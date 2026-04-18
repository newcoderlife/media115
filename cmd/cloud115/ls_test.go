package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

func TestLsDirPrintShortMode(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "folderA", Type: "dir"},
		{Name: "movie.mkv", Type: "file", Size: 1024 * 1024 * 100},
		{Name: "show.mp4", Type: "file", Size: 2048},
	}

	var buf bytes.Buffer
	lsDirPrint(&buf, nil, items, "/", false, false, 0, 0)
	output := buf.String()

	if !strings.Contains(output, "folderA/") {
		t.Errorf("expected 'folderA/' in output, got: %q", output)
	}
	if !strings.Contains(output, "movie.mkv") {
		t.Errorf("expected 'movie.mkv' in output, got: %q", output)
	}
	if !strings.Contains(output, "show.mp4") {
		t.Errorf("expected 'show.mp4' in output, got: %q", output)
	}
}

func TestLsDirPrintLongMode(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "docs", Type: "dir"},
		{Name: "readme.txt", Type: "file", Size: 512},
	}

	var buf bytes.Buffer
	lsDirPrint(&buf, nil, items, "/", true, false, 0, 0)
	output := buf.String()

	if !strings.Contains(output, "d") || !strings.Contains(output, "docs/") {
		t.Errorf("long mode: expected 'd  ... docs/' in output, got: %q", output)
	}
	if !strings.Contains(output, "f") || !strings.Contains(output, "readme.txt") {
		t.Errorf("long mode: expected 'f  ... readme.txt' in output, got: %q", output)
	}
	if !strings.Contains(output, "512B") {
		t.Errorf("long mode: expected '512B' in output, got: %q", output)
	}
}

func TestLsDirPrintIndent(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "sub", Type: "dir"},
	}

	var buf0, buf2 bytes.Buffer
	lsDirPrint(&buf0, nil, items, "/", false, false, 0, 0)
	lsDirPrint(&buf2, nil, items, "/", false, false, 0, 2)
	output0 := buf0.String()
	output2 := buf2.String()

	if strings.HasPrefix(output0, " ") {
		t.Errorf("indent=0 should not have leading space, got: %q", output0)
	}
	if !strings.HasPrefix(output2, "    ") {
		t.Errorf("indent=2 should have 4-space prefix, got: %q", output2)
	}
}

func TestLsDirPrintEmpty(t *testing.T) {
	var buf bytes.Buffer
	lsDirPrint(&buf, nil, []cloud115.Entry{}, "/", false, false, 0, 0)
	if buf.String() != "" {
		t.Errorf("empty items should produce no output, got: %q", buf.String())
	}
}

func TestLsDirPrintLongDirNoSize(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "mydir", Type: "dir"},
	}

	var buf bytes.Buffer
	lsDirPrint(&buf, nil, items, "/", true, false, 0, 0)
	output := buf.String()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "d") {
		t.Errorf("expected 'd' type in long listing, got: %q", lines[0])
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[0]), "mydir/") {
		t.Errorf("dir in long mode should end with '/', got: %q", lines[0])
	}
}

func TestLsDirPrintMultipleFiles(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "a.txt", Type: "file", Size: 100},
		{Name: "b.txt", Type: "file", Size: 200},
		{Name: "c.txt", Type: "file", Size: 300},
	}

	var buf bytes.Buffer
	lsDirPrint(&buf, nil, items, "/", false, false, 0, 0)
	output := buf.String()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), output)
	}
}

type mockLsClient struct {
	listDirFn func(path string) ([]cloud115.Entry, error)
}

func (m *mockLsClient) ListDir(path string) ([]cloud115.Entry, error) { return m.listDirFn(path) }
func (m *mockLsClient) Close() error                                  { return nil }

func TestLsRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *lsOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &lsOpts{
				Path:      "/",
				GetClient: func() (lsClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "list error",
			opts: &lsOpts{
				Path: "/missing",
				GetClient: func() (lsClient, error) {
					return &mockLsClient{
						listDirFn: func(_ string) ([]cloud115.Entry, error) { return nil, errors.New("not found") },
					}, nil
				},
			},
			wantErr: "目录不存在",
		},
		{
			name: "success",
			opts: &lsOpts{
				Path: "/",
				GetClient: func() (lsClient, error) {
					return &mockLsClient{
						listDirFn: func(_ string) ([]cloud115.Entry, error) {
							return []cloud115.Entry{{Name: "test.txt", Type: "file"}}, nil
						},
					}, nil
				},
			},
			wantOut: "test.txt",
		},
		{
			name: "recursive",
			opts: &lsOpts{
				Path:      "/",
				Recursive: true,
				Depth:     1,
				GetClient: func() (lsClient, error) {
					return &mockLsClient{
						listDirFn: func(path string) ([]cloud115.Entry, error) {
							if path == "/" {
								return []cloud115.Entry{{Name: "sub", Type: "dir"}}, nil
							}
							return []cloud115.Entry{{Name: "child.txt", Type: "file"}}, nil
						},
					}, nil
				},
			},
			wantOut: "child.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := lsRun(tt.opts)
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
