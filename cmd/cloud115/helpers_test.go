package main

import (
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{"zero bytes", 0, "0B"},
		{"1 byte", 1, "1B"},
		{"999 bytes", 999, "999B"},
		{"1023 bytes", 1023, "1023B"},
		{"1 KB exact", 1024, "1.0K"},
		{"1.5 KB", 1536, "1.5K"},
		{"999.9 KB", 1023*1024 + 512, "1023.5K"},
		{"1 MB exact", 1024 * 1024, "1.0M"},
		{"2.5 MB", int64(2.5 * 1024 * 1024), "2.5M"},
		{"1 GB exact", 1024 * 1024 * 1024, "1.0G"},
		{"1.2 GB", int64(1288490188), "1.2G"}, // 1.2 * 1024^3
		{"1 TB exact", 1024 * 1024 * 1024 * 1024, "1.0T"},
		{"large petabytes", 1024 * 1024 * 1024 * 1024 * 1024 * 2, "2.0P"}, // 2*1024^5 / 1024^5 = 2.0P
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSize(tc.input)
			if got != tc.want {
				t.Errorf("formatSize(%d) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatSizeUnits(t *testing.T) {
	// Boundary: exactly 1024 should switch to next unit
	got := formatSize(1024)
	if got != "1.0K" {
		t.Errorf("formatSize(1024) = %q, want %q", got, "1.0K")
	}

	got = formatSize(1024 * 1024)
	if got != "1.0M" {
		t.Errorf("formatSize(1024*1024) = %q, want %q", got, "1.0M")
	}

	got = formatSize(1024 * 1024 * 1024)
	if got != "1.0G" {
		t.Errorf("formatSize(1024*1024*1024) = %q, want %q", got, "1.0G")
	}

	got = formatSize(1024 * 1024 * 1024 * 1024)
	if got != "1.0T" {
		t.Errorf("formatSize(1024^4) = %q, want %q", got, "1.0T")
	}
}
