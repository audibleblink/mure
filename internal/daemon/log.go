package daemon

import (
	"os"
	"path/filepath"
	"sync"
)

// LogMaxBytes is the rotation threshold (PRD §6.6: 4MB, one .1 backup).
const LogMaxBytes = 4 * 1024 * 1024

// Logger is a tiny rotating file writer. It is io.Writer-compatible so it can
// back a log.Logger or be written to directly.
type Logger struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
	max  int64
}

// NewLogger opens (or creates) <dir>/daemon.log for appending.
func NewLogger(dir string) (*Logger, error) {
	return NewLoggerWithLimit(dir, LogMaxBytes)
}

// NewLoggerWithLimit lets callers override the rotation threshold (for tests).
func NewLoggerWithLimit(dir string, max int64) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "daemon.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Logger{path: p, f: f, size: st.Size(), max: max}, nil
}

// Write appends to the log, rotating first if the new write would exceed max.
func (l *Logger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(p)) > l.max {
		if err := l.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

func (l *Logger) rotateLocked() error {
	if err := l.f.Close(); err != nil {
		return err
	}
	_ = os.Remove(l.path + ".1")
	if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	l.f = f
	l.size = 0
	return nil
}

// Close releases the underlying file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
