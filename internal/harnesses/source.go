package harnesses

import (
	"io/fs"
	"os"
)

var testFS fs.FS // set by SetFSForTesting

// SetFSForTesting overrides the harness FS for the duration of a test.
// Pass nil to restore default.
func SetFSForTesting(f fs.FS) { testFS = f }

// SourceFS returns the harness FS used by the CLI: the test override (if any),
// else $MURE_HARNESSES_DIR (if set, as os.DirFS), else the embedded FS.
func SourceFS() fs.FS {
	if testFS != nil {
		return testFS
	}
	if d := os.Getenv("MURE_HARNESSES_DIR"); d != "" {
		return os.DirFS(d)
	}
	return FS()
}
