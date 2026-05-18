// Package piext exposes the embedded pi extension assets, synced from
// pi-mure/ by `make sync-piext`.
package piext

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var assets embed.FS

// FS returns the embedded asset tree rooted at "assets/".
func FS() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
