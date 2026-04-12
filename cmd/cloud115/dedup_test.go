package main

import "testing"

func TestTrimSlash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no slash", "foo", "foo"},
		{"single trailing slash", "foo/", "foo"},
		{"multiple trailing slashes", "foo///", "foo"},
		{"root slash kept", "/", "/"},
		{"nested path trailing slash", "/a/b/c/", "/a/b/c"},
		{"nested path multiple trailing slashes", "/a/b/c///", "/a/b/c"},
		{"empty string", "", ""},
		{"single slash stays", "/", "/"},
		{"path no trailing slash unchanged", "/a/b", "/a/b"},
		{"only slashes single char", "/", "/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := trimSlash(tc.input)
			if got != tc.want {
				t.Errorf("trimSlash(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTrimSlashPreservesRoot(t *testing.T) {
	// The root "/" should never be trimmed to empty string.
	result := trimSlash("/")
	if result != "/" {
		t.Errorf("trimSlash('/') = %q, want '/'", result)
	}
}

func TestTrimSlashIdempotent(t *testing.T) {
	inputs := []string{"/foo/bar", "baz", "/", "hello/world"}
	for _, in := range inputs {
		first := trimSlash(in)
		second := trimSlash(first)
		if first != second {
			t.Errorf("trimSlash not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}
