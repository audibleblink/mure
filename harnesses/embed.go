// Package harnessfs holds the embedded harness manifests and assets.
// It lives next to the harness data so go:embed can include the tree
// without escaping its source directory.
package harnessfs

import (
	"embed"
	"io/fs"
)

//go:embed all:*
var embedded embed.FS

// FS returns the embedded harnesses tree.
func FS() fs.FS { return embedded }
