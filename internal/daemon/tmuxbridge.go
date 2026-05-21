package daemon

import (
	"context"
	"fmt"

	"github.com/audibleblink/mure/internal/tmuxctl"
)

// Bridge wires tmux control-mode events into the daemon: pane-died events
// remove the corresponding agent from the roster. It does NOT write any
// per-pane @mure-* options; agent state is observable only via the daemon
// socket (`mure ls`) and the sidebar that reads from it.
type Bridge struct {
	tmux    tmuxctl.Client
	roster  *Roster
	session string
}

// NewBridge constructs a Bridge. Caller owns the tmux client lifetime.
func NewBridge(tmux tmuxctl.Client, roster *Roster, session string) *Bridge {
	return &Bridge{tmux: tmux, roster: roster, session: session}
}

// SetupSession publishes MURE_* into the tmux session environment and tells
// tmux to suppress %output noise.
func (b *Bridge) SetupSession(ctx context.Context, runDir, sockPath string) error {
	cmds := []string{
		fmt.Sprintf("set-environment -t %s MURE_ENV 1", b.session),
		fmt.Sprintf("set-environment -t %s MURE_SESSION %s", b.session, b.session),
		fmt.Sprintf("set-environment -t %s MURE_RUN_DIR %s", b.session, runDir),
		fmt.Sprintf("set-environment -t %s MURE_SOCKET %s", b.session, sockPath),
	}
	for _, c := range cmds {
		if _, err := b.tmux.Run(ctx, c); err != nil {
			return fmt.Errorf("tmux %q: %w", c, err)
		}
	}
	if _, err := b.tmux.Run(ctx, "refresh-client -f no-output"); err != nil {
		return fmt.Errorf("tmux refresh-client: %w", err)
	}
	return nil
}

// Run drives the bridge until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) {
	events := b.tmux.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Kind == tmuxctl.EventPaneDied {
				for _, a := range b.roster.Snapshot().Agents {
					if a.Pane == ev.PaneID {
						b.roster.Remove(a.ID)
					}
				}
			}
		}
	}
}
