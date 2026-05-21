package harnesses

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
)

// Load walks root looking for "<name>/manifest.toml" files, decodes each
// strictly, and returns the resulting manifests sorted by Name.
// Errors are aggregated across all manifests; the path is included in messages.
func Load(root fs.FS) ([]Manifest, error) {
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	var manifests []Manifest
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := path.Join(e.Name(), "manifest.toml")
		data, err := fs.ReadFile(root, p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		m, err := DecodeManifest(data)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		manifests = append(manifests, m)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	return manifests, errors.Join(errs...)
}

// Get returns the manifest with the given Name and true, or zero/false if not found.
func Get(manifests []Manifest, name string) (Manifest, bool) {
	for _, m := range manifests {
		if m.Name == name {
			return m, true
		}
	}
	return Manifest{}, false
}
