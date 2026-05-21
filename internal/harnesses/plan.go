package harnesses

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
)

// FileOp describes a single file write produced by BuildPlan.
type FileOp struct {
	Dst     string      // absolute destination path (after ~ expansion)
	Mode    fs.FileMode // file mode
	Content []byte      // file contents
	Merge   string      // "append" | "replace" | "create-if-missing"
}

// expandHome expands a leading "~/" or "~" in p using $HOME.
func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return path.Join(home, p[2:]), nil
	}
	return p, nil
}

// parseMode parses an octal mode string like "0755"; defaults to 0644 when empty.
func parseMode(s string) (fs.FileMode, error) {
	if s == "" {
		return 0o644, nil
	}
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q: %w", s, err)
	}
	return fs.FileMode(n), nil
}

// BuildPlan materializes a manifest's skill + hooks into FileOps.
// root is the harness FS rooted at the harnesses tree (so the manifest's
// own files live at "<name>/...").
func BuildPlan(m Manifest, root fs.FS) ([]FileOp, error) {
	var ops []FileOp
	if m.Install.Skill.Path != "" {
		dst, err := expandHome(m.Install.Skill.Path)
		if err != nil {
			return nil, err
		}
		content, err := fs.ReadFile(root, path.Join(m.Name, "skill.md"))
		if err != nil {
			return nil, fmt.Errorf("read skill: %w", err)
		}
		ops = append(ops, FileOp{
			Dst:     dst,
			Mode:    0o644,
			Content: content,
			Merge:   m.Install.Skill.Merge,
		})
	}
	for _, f := range m.Install.Files {
		dst, err := expandHome(f.Dst)
		if err != nil {
			return nil, err
		}
		mode, err := parseMode(f.Mode)
		if err != nil {
			return nil, err
		}
		content, err := fs.ReadFile(root, path.Join(m.Name, f.Src))
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", f.Src, err)
		}
		ops = append(ops, FileOp{
			Dst:     dst,
			Mode:    mode,
			Content: content,
			Merge:   "replace",
		})
	}
	return ops, nil
}
