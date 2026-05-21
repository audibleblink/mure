package daemon

import (
	"context"
	"fmt"
	"log"

	"github.com/audibleblink/mure/internal/tmuxctl"
)

// Config is the assembly-time configuration for Run.
type Config struct {
	Session    string
	SocketPath string
	RunDir     string
	LaunchDir  string // cwd of the process that ran `mure up`
	Tmux       tmuxctl.Client
}

// Run wires the daemon's subsystems together: logger, roster, tmux bridge,
// and Unix-socket server. Returns when ctx is cancelled or the listener errors.
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

	if cfg.Tmux != nil {
		bridge := NewBridge(cfg.Tmux, roster, cfg.Session, l)
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
