package harnesses

import (
	"bytes"
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// Manifest mirrors §7 of PRD 005.
type Manifest struct {
	ManifestVersion int          `toml:"manifest_version"`
	Name            string       `toml:"name"`
	Display         string       `toml:"display"`
	Command         string       `toml:"command"`
	TaskArg         string       `toml:"task_arg"`
	Capabilities    Capabilities `toml:"capabilities"`
	Install         Install      `toml:"install"`
}

type Capabilities struct {
	Spawn    bool `toml:"spawn"`
	Status   bool `toml:"status"`
	Result   bool `toml:"result"`
	Subtools bool `toml:"subtools"`
}

type Install struct {
	Skill Skill  `toml:"skill"`
	Files []File `toml:"files"`
}

type Skill struct {
	Path  string `toml:"path"`
	Merge string `toml:"merge"`
}

// File is a single embedded-source → on-disk destination copy operation.
// Used for hook scripts, plugins, or any other file a harness needs to
// drop into its agent's config tree at install time.
type File struct {
	Src  string `toml:"src"`
	Dst  string `toml:"dst"`
	Mode string `toml:"mode"`
}

var validMerge = map[string]bool{
	"":                 true, // unset allowed when Skill is empty
	"append":           true,
	"replace":          true,
	"create-if-missing": true,
}

// DecodeManifest strictly decodes a TOML manifest, validates required fields,
// task_arg, and merge mode. Unknown keys produce an error.
func DecodeManifest(data []byte) (Manifest, error) {
	var m Manifest
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, err
	}
	if m.Name == "" {
		return m, fmt.Errorf("manifest: missing required field 'name'")
	}
	if m.Command == "" {
		return m, fmt.Errorf("manifest: missing required field 'command'")
	}
	if _, err := ParseTaskArg(m.TaskArg); err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	if m.Install.Skill.Path != "" {
		if !validMerge[m.Install.Skill.Merge] || m.Install.Skill.Merge == "" {
			return m, fmt.Errorf("manifest: invalid skill merge mode %q", m.Install.Skill.Merge)
		}
	} else if m.Install.Skill.Merge != "" && !validMerge[m.Install.Skill.Merge] {
		return m, fmt.Errorf("manifest: invalid skill merge mode %q", m.Install.Skill.Merge)
	}
	return m, nil
}
