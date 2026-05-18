package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/audibleblink/mure/internal/tmuxctl"
)

// Config is the assembly-time configuration for Run.
type Config struct {
	Session        string
	SocketPath     string
	RunDir         string
	LaunchDir      string // cwd of the process that ran `mure up`
	Reader         tmuxctl.Client
	Writer         tmuxctl.Client
	CoalesceWindow time.Duration // 0 → DefaultCoalesceWindow
}

// Run wires the daemon's subsystems together: logger, roster, coalescer,
// debouncer, tmux bridge, and Unix-socket server. Returns when ctx is
// cancelled or the listener errors.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Session == "" || cfg.SocketPath == "" || cfg.RunDir == "" {
		return fmt.Errorf("daemon: Config requires Session, SocketPath, RunDir")
	}
	logger, err := NewLogger(cfg.RunDir)
	if err != nil {
		return err
	}
	defer logger.Close()
	l := log.New(logger, "", log.LstdFlags|log.Lmicroseconds)
	l.Printf("daemon: starting session=%s socket=%s rundir=%s", cfg.Session, cfg.SocketPath, cfg.RunDir)

	roster := NewRoster()
	defer roster.Close()
	if cfg.LaunchDir != "" {
		roster.SetLaunchDir(cfg.LaunchDir)
	}
	coalWin := cfg.CoalesceWindow
	if coalWin <= 0 {
		coalWin = DefaultCoalesceWindow
	}
	coal := NewCoalescer(coalWin)
	defer coal.Close()
	deb := NewDebouncer(DefaultDebounceWindow,
		func(id string) { roster.Remove(id) },
		func(id string) { roster.MarkDisconnected(id, 0) },
	)
	defer deb.Stop()

	if cfg.Reader != nil && cfg.Writer != nil {
		bridge := NewBridge(cfg.Reader, cfg.Writer, roster, coal, deb, cfg.Session)
		if err := bridge.SetupSession(ctx, cfg.RunDir, cfg.SocketPath); err != nil {
			l.Printf("setup-session: %v", err)
			return err
		}
		go bridge.Run(ctx)
	}

	srv, err := Listen(cfg.SocketPath, roster)
	if err != nil {
		l.Printf("listen: %v", err)
		return err
	}
	l.Printf("daemon: listening")
	return srv.Run(ctx)
}
