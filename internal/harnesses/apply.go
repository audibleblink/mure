package harnesses

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileReceipt records a single applied file's outcome.
type FileReceipt struct {
	Dst   string      `json:"dst"`
	Mode  fs.FileMode `json:"mode"`
	SHA   string      `json:"sha256"`
	Merge string      `json:"merge"`
}

// Receipt is the per-harness install record.
type Receipt struct {
	Harness string        `json:"harness"`
	Files   []FileReceipt `json:"files"`
}

// Apply executes ops. For "append" the dst is read, ReplaceOrAppendBlock
// rewrites the per-harness block. For "create-if-missing" existing files
// are left alone. Anything else (incl. "" and "replace") overwrites.
// harness identifies the marker block name for append merges.
func Apply(harness string, ops []FileOp) (Receipt, error) {
	r := Receipt{Harness: harness}
	for _, op := range ops {
		if err := os.MkdirAll(filepath.Dir(op.Dst), 0o755); err != nil {
			return r, err
		}
		merge := op.Merge
		if merge == "" {
			merge = "replace"
		}
		var written []byte
		switch merge {
		case "create-if-missing":
			if _, err := os.Stat(op.Dst); err == nil {
				// Skip but still record current contents' hash.
				b, _ := os.ReadFile(op.Dst)
				written = b
			} else if errors.Is(err, fs.ErrNotExist) {
				if err := os.WriteFile(op.Dst, op.Content, op.Mode); err != nil {
					return r, err
				}
				written = op.Content
			} else {
				return r, err
			}
		case "append":
			existing, err := readIfExists(op.Dst)
			if err != nil {
				return r, err
			}
			updated := ReplaceOrAppendBlock(existing, harness, string(op.Content))
			if err := os.WriteFile(op.Dst, []byte(updated), op.Mode); err != nil {
				return r, err
			}
			written = []byte(updated)
		case "replace":
			if err := os.WriteFile(op.Dst, op.Content, op.Mode); err != nil {
				return r, err
			}
			written = op.Content
		default:
			return r, fmt.Errorf("unknown merge mode %q", merge)
		}
		r.Files = append(r.Files, FileReceipt{
			Dst:   op.Dst,
			Mode:  op.Mode,
			SHA:   sum(written),
			Merge: merge,
		})
	}
	return r, nil
}

func readIfExists(p string) (string, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
