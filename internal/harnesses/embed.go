package harnesses

import (
	"io/fs"

	harnessfs "github.com/audibleblink/mure/harnesses"
)

// FS returns the embedded harnesses tree.
func FS() fs.FS { return harnessfs.FS() }
