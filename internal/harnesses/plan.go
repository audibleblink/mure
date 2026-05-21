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

// expandPath expands environment variables and a leading "~" in p.
// Supports ${VAR} and ${VAR:-default} (default may itself contain ~ or $VARs,
// recursively). $VAR (no braces) is also supported via os.Expand semantics.
func expandPath(p string) (string, error) {
	p = expandEnvDefaults(p)
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

// expandEnvDefaults expands ${VAR} and ${VAR:-default} forms, plus bare $VAR.
// For ${VAR:-default} the default is itself recursively expanded.
func expandEnvDefaults(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); {
		if p[i] != '$' {
			b.WriteByte(p[i])
			i++
			continue
		}
		// ${...}
		if i+1 < len(p) && p[i+1] == '{' {
			end := strings.IndexByte(p[i+2:], '}')
			if end < 0 {
				b.WriteByte(p[i])
				i++
				continue
			}
			expr := p[i+2 : i+2+end]
			i += 2 + end + 1
			name, def, hasDef := expr, "", false
			if idx := strings.Index(expr, ":-"); idx >= 0 {
				name, def, hasDef = expr[:idx], expr[idx+2:], true
			}
			if v, ok := os.LookupEnv(name); ok && v != "" {
				b.WriteString(v)
			} else if hasDef {
				b.WriteString(expandEnvDefaults(def))
			}
			continue
		}
		// $VAR
		j := i + 1
		for j < len(p) && (p[j] == '_' || (p[j] >= 'A' && p[j] <= 'Z') || (p[j] >= 'a' && p[j] <= 'z') || (p[j] >= '0' && p[j] <= '9')) {
			j++
		}
		if j == i+1 {
			b.WriteByte(p[i])
			i++
			continue
		}
		b.WriteString(os.Getenv(p[i+1 : j]))
		i = j
	}
	return b.String()
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
		dst, err := expandPath(m.Install.Skill.Path)
		if err != nil {
			return nil, err
		}
		content, err := fs.ReadFile(root, path.Join(m.Name, "SKILL.md"))
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
		dst, err := expandPath(f.Dst)
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
