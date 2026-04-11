package organizer

import (
	"github.com/newcoderlife/media115/internal/cloud115"
)

// Verify checks that organize results are as expected by refreshing touched
// directories and confirming each op's target file exists.
//
// catPath must start with "/" (e.g. "/影音/AV").
// Returns (verified, mismatches).
func Verify(client *cloud115.Client, ops []Op, catPath string) (verified, mismatches int) {
	// Collect unique directories that were touched.
	touched := map[string]bool{}
	for _, op := range ops {
		if op.NewFolder != "" {
			touched[catPath+"/"+op.NewFolder] = true
		} else {
			touched["/"+op.Parent] = true
		}
	}
	if len(touched) == 0 {
		return 0, 0
	}

	// Refresh all touched dirs in one call.
	paths := make([]string, 0, len(touched))
	for p := range touched {
		paths = append(paths, p)
	}
	_ = client.RefreshPaths(paths)

	// Verify each op's expected file is present.
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

		entries, err := client.ListDir(targetDir)
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
