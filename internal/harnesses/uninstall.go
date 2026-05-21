package harnesses

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Uninstall reverses an Apply receipt. For append-mode files the marker
// block is stripped. For replace/create-if-missing the file is deleted only
// if its sha256 still matches the receipt; otherwise a warning is returned
// in the error list (but uninstall continues).
func Uninstall(r Receipt) []error {
	var warns []error
	for _, f := range r.Files {
		switch f.Merge {
		case "append":
			b, err := os.ReadFile(f.Dst)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				warns = append(warns, err)
				continue
			}
			updated := StripBlock(string(b), r.Harness)
			if updated == "" {
				if err := os.Remove(f.Dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
					warns = append(warns, err)
				}
				continue
			}
			if err := os.WriteFile(f.Dst, []byte(updated), f.Mode); err != nil {
				warns = append(warns, err)
			}
		case "replace", "create-if-missing":
			cur, err := os.ReadFile(f.Dst)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				warns = append(warns, err)
				continue
			}
			if sum(cur) != f.SHA {
				warns = append(warns, fmt.Errorf("skipping modified file: %s", f.Dst))
				continue
			}
			if err := os.Remove(f.Dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
				warns = append(warns, err)
			}
		}
	}
	return warns
}
