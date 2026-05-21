package daemon

import (
	"context"
	"fmt"

	"github.com/audibleblink/mure/internal/tmuxctl"
)

// Bridge wires tmux control-mode events (pane-died, etc.) into the daemon's
// roster lifecycle. It does NOT write any per-pane @mure-* status options:
// agent status is observable only via the daemon socket (`mure ls`) and the
// sidebar that reads from it.
type Bridge struct {
	reader  tmuxctl.Client
	writer  tmuxctl.Client
	roster  *Roster
	deb     *Debouncer
	session string
}

// NewBridge constructs a Bridge. Caller owns lifetimes of all dependencies.
func NewBridge(reader, writer tmuxctl.Client, roster *Roster, deb *Debouncer, session string) *Bridge {
	return &Bridge{reader: reader, writer: writer, roster: roster, deb: deb, session: session}
}

// SetupSession publishes MURE_* into the tmux session environment and tells
// the reader client to suppress %output.
func (b *Bridge) SetupSession(ctx context.Context, runDir, sockPath string) error {
	cmds := []string{
		fmt.Sprintf("set-environment -t %s MURE_ENV 1", b.session),
		fmt.Sprintf("set-environment -t %s MURE_SESSION %s", b.session, b.session),
		fmt.Sprintf("set-environment -t %s MURE_RUN_DIR %s", b.session, runDir),
		fmt.Sprintf("set-environment -t %s MURE_SOCKET %s", b.session, sockPath),
	}
	for _, c := range cmds {
		if _, err := b.writer.Run(ctx, c); err != nil {
			return fmt.Errorf("tmux %q: %w", c, err)
		}
	}
	if _, err := b.reader.Run(ctx, "refresh-client -f no-output"); err != nil {
		return fmt.Errorf("tmux refresh-client: %w", err)
	}
	return nil
}

// Run drives the bridge until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) {
	events := b.reader.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			b.handleEvent(ev)
		}
	}
}

func (b *Bridge) handleEvent(ev tmuxctl.Event) {
	switch ev.Kind {
	case tmuxctl.EventPaneDied:
		for _, a := range b.roster.Snapshot().Agents {
			if a.Pane == ev.PaneID {
				b.deb.OnPaneDied(a.ID)
			}
		}
	case tmuxctl.EventWindowClose, tmuxctl.EventSessionWindowChanged:
		// Reserved for future per-window teardown logic.
	}
}
