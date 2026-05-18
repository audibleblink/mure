package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/audibleblink/mure/internal/sock"
	"github.com/audibleblink/mure/internal/tmuxctl"
)

// Bridge glues two tmuxctl.Client instances (reader + writer per PRD §6.1)
// to the daemon: tmux events drive roster transitions; roster transitions
// fan out (via Coalescer) into per-pane @mure-* option writes.
type Bridge struct {
	reader  tmuxctl.Client
	writer  tmuxctl.Client
	roster  *Roster
	coal    *Coalescer
	deb     *Debouncer
	session string
}

// NewBridge constructs a Bridge. Caller owns lifetimes of all dependencies.
func NewBridge(reader, writer tmuxctl.Client, roster *Roster, coal *Coalescer, deb *Debouncer, session string) *Bridge {
	return &Bridge{reader: reader, writer: writer, roster: roster, coal: coal, deb: deb, session: session}
}

// SetupSession publishes MURE_* into the tmux session environment and tells
// the reader client to suppress %output (PRD §6.1).
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

// Run drives the bridge until ctx is cancelled. It blocks; typical use is
// `go bridge.Run(ctx)`.
func (b *Bridge) Run(ctx context.Context) {
	updates, cancel := b.roster.Subscribe()
	defer cancel()
	go b.writerLoop(ctx)

	events := b.reader.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			b.handleEvent(ev)
		case upd, ok := <-updates:
			if !ok {
				return
			}
			b.handleUpdate(upd)
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
		// Window-level teardown: pane-died fires per pane, so we rely on
		// that. Reserved hook for future per-window removal logic.
	}
}

func (b *Bridge) handleUpdate(upd sock.AgentUpdate) {
	if upd.Agent.Pane == "" {
		return
	}
	b.coal.Submit(upd.Agent.Pane, "@mure-status", upd.Agent.Status)
	if upd.Agent.Task != "" {
		b.coal.Submit(upd.Agent.Pane, "@mure-task", upd.Agent.Task)
	}
}

func (b *Bridge) writerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case w, ok := <-b.coal.Out():
			if !ok {
				return
			}
			val := w.Value
			if strings.HasPrefix(w.Option, "@mure-") {
				// Escape '#' so tmux does not re-expand format directives
				// (e.g. '#(...)', '#{...}') when these options are interpolated
				// into pane-border-format / status-right. See tmux-mure README
				// "Daemon contract".
				val = strings.ReplaceAll(val, "#", "##")
			}
			cmd := fmt.Sprintf("set-option -p -t %s %s %s", w.PaneID, w.Option, shellQuote(val))
			_, _ = b.writer.Run(ctx, cmd)
		}
	}
}

// shellQuote wraps s in single quotes for tmux, escaping any embedded single
// quotes. tmux's command parser is shell-like; the values we write are short
// status tokens / task names, so simple single-quote escaping is sufficient.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\\', '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	out = append(out, '\'')
	return string(out)
}
