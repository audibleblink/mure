package harnesses

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// StateDir returns the directory under which integration receipts are stored.
// Honors $XDG_STATE_HOME; otherwise falls back to ~/.local/state.
func StateDir() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "mure", "integrations"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "mure", "integrations"), nil
}

func statePath(name string) (string, error) {
	d, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name+".json"), nil
}

// WriteState persists r under <state>/<name>.json.
func WriteState(name string, r Receipt) error {
	p, err := statePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// ReadState returns the receipt and true if present, or zero/false if absent.
func ReadState(name string) (Receipt, bool, error) {
	p, err := statePath(name)
	if err != nil {
		return Receipt{}, false, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	var r Receipt
	if err := json.Unmarshal(b, &r); err != nil {
		return Receipt{}, false, err
	}
	return r, true, nil
}

// ClearState removes the state file for name (no error if absent).
func ClearState(name string) error {
	p, err := statePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
