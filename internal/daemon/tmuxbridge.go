package daemon

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/audibleblink/mure/internal/tmuxctl"
)

// pollInterval is how often the bridge reconciles its roster against the
// live tmux pane list. Tmux does not emit a control-mode notification when
// a pane dies (the `%pane-died` / `%pane-exited` lines listed in some docs
// don't actually fire in tmux 3.6a for normal `kill-pane` or process exit),
// so polling is the only reliable way to detect pane disappearance.
const pollInterval = 1 * time.Second

// Bridge reconciles the daemon's agent roster against tmux's live pane
// list. Agents whose pane has disappeared are removed.
type Bridge struct {
	tmux    tmuxctl.Client
	roster  *Roster
	session string
	log     *log.Logger
}

// NewBridge constructs a Bridge. Caller owns the tmux client lifetime.
// A nil logger disables prune logging.
func NewBridge(tmux tmuxctl.Client, roster *Roster, session string, logger *log.Logger) *Bridge {
	return &Bridge{tmux: tmux, roster: roster, session: session, log: logger}
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

// Run drives the bridge until ctx is cancelled, polling tmux for the live
// pane list and pruning the roster on each tick.
func (b *Bridge) Run(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.reconcile(ctx)
		}
	}
}

// reconcile removes any agent whose pane id is no longer present in tmux.
// A list-panes failure is treated as transient and ignored — better to keep
// stale agents one tick than to drop everyone on a flaky tmux call.
func (b *Bridge) reconcile(ctx context.Context) {
	// Prefix the format with a literal sentinel so payload lines never
	// start with '%' (pane ids do). This is defensive — the tmuxctl reader
	// is also fixed to keep '%'-prefixed payload lines inside a reply
	// block — but it costs nothing and removes the foot-gun for future
	// callers.
	const prefix = "p="
	out, err := b.tmux.Run(ctx, "list-panes -aF '"+prefix+"#{pane_id}'")
	if err != nil {
		return
	}
	live := make(map[string]struct{}, 16)
	for _, ln := range strings.Split(out, "\n") {
		id := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), prefix))
		if id != "" {
			live[id] = struct{}{}
		}
	}
	for _, a := range b.roster.Snapshot().Agents {
		if a.Pane == "" {
			continue
		}
		if _, ok := live[a.Pane]; !ok {
			if b.log != nil {
				b.log.Printf("bridge: prune agent=%s pane=%s (not in tmux)", a.ID, a.Pane)
			}
			b.roster.Remove(a.ID)
		}
	}
}
