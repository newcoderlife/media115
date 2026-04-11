package organizer

import (
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// stubClient implements the minimum interface needed by Verify.
// It stores pre-loaded directory listings.
type stubClient struct {
	dirs map[string][]cloud115.Entry
}

func (s *stubClient) ListDir(path string) ([]cloud115.Entry, error) {
	if entries, ok := s.dirs[path]; ok {
		return entries, nil
	}
	return nil, &notFoundError{path}
}

func (s *stubClient) RefreshPaths(paths []string) error { return nil }

type notFoundError struct{ path string }

func (e *notFoundError) Error() string { return "not found: " + e.path }

// verifyWithStub is a test-only version of Verify that accepts a stub client.
func verifyWithStub(stub *stubClient, ops []Op, catPath string) (verified, mismatches int) {
	for _, op := range ops {
		expectedName := op.File
		if op.NewName != "" {
			expectedName = op.NewName
		}
		var targetDir string
		if op.NewFolder != "" {
			targetDir = catPath + "/" + op.NewFolder
		} else {
			targetDir = "/" + op.Parent
		}
		entries, err := stub.ListDir(targetDir)
		if err != nil {
			mismatches++
			continue
		}
		found := false
		for _, e := range entries {
			if e.Name == expectedName {
				found = true
				break
			}
		}
		if found {
			verified++
		} else {
			mismatches++
		}
	}
	return verified, mismatches
}

func TestVerifyAllPresent(t *testing.T) {
	ops := []Op{
		{File: "Inception.2010.mkv", NewName: "Inception (2010).mkv", NewFolder: "Inception (2010)", Parent: "影音/电影/old"},
		{File: "T (2025).mkv", NewName: "", NewFolder: "", Parent: "影音/电影/T (2025)"},
	}
	stub := &stubClient{
		dirs: map[string][]cloud115.Entry{
			"/影音/AV/Inception (2010)": {
				{Name: "Inception (2010).mkv", Type: "file"},
			},
			"/影音/AV/T (2025)": {
				{Name: "T (2025).mkv", Type: "file"},
			},
			// catPath based lookups:
			"/影音/电影/Inception (2010)": {
				{Name: "Inception (2010).mkv", Type: "file"},
			},
			"/影音/电影/T (2025)": {
				{Name: "T (2025).mkv", Type: "file"},
			},
		},
	}
	v, m := verifyWithStub(stub, ops, "/影音/电影")
	if v != 2 || m != 0 {
		t.Errorf("expected (2,0), got (%d,%d)", v, m)
	}
}

func TestVerifyMismatch(t *testing.T) {
	ops := []Op{
		{File: "Wrong.mkv", NewName: "Correct.mkv", NewFolder: "Correct (2020)", Parent: "影音/电影/orig"},
	}
	stub := &stubClient{
		dirs: map[string][]cloud115.Entry{
			// Dir exists but file has different name (move failed or wrong name).
			"/影音/电影/Correct (2020)": {
				{Name: "Wrong.mkv", Type: "file"}, // still old name
			},
		},
	}
	v, m := verifyWithStub(stub, ops, "/影音/电影")
	if v != 0 || m != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", v, m)
	}
}

func TestVerifyMissingDir(t *testing.T) {
	ops := []Op{
		{File: "Something.mkv", NewName: "Something (2020).mkv", NewFolder: "Something (2020)", Parent: "影音/电影/orig"},
	}
	stub := &stubClient{dirs: map[string][]cloud115.Entry{}} // empty
	v, m := verifyWithStub(stub, ops, "/影音/电影")
	if v != 0 || m != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", v, m)
	}
}
