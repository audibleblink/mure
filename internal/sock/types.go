// Package sock defines the daemon wire protocol (PRD §12).
package sock

// ProtocolVersion is the v1 wire version. Daemons reject any other value.
const ProtocolVersion = 1

// MaxFrameSize is the maximum NDJSON line length in bytes (PRD §12).
const MaxFrameSize = 64 * 1024

// Status vocabulary (PRD §12.5).
const (
	StatusIdle         = "idle"
	StatusWorking      = "working"
	StatusBlocked      = "blocked"
	StatusDisconnected = "disconnected"
	StatusErrored      = "errored"
)

// ValidStatus reports whether s is a known status string.
func ValidStatus(s string) bool {
	switch s {
	case StatusIdle, StatusWorking, StatusBlocked, StatusDisconnected, StatusErrored:
		return true
	}
	return false
}

// Connection roles (first-frame hello.role, PRD §12).
const (
	RoleAgent   = "agent"
	RoleSidebar = "sidebar"
	RoleCLI     = "cli"
	RoleHook    = "hook"
)

// Hello is the first frame sent by every connection.
// Agent connections populate AgentID/PaneID/PID/PiVersion/TS; hook/cli/sidebar
// connections send only V/Event/Role.
type Hello struct {
	V         int    `json:"v"`
	Event     string `json:"event"`
	Role      string `json:"role"`
	AgentID   string `json:"agent_id,omitempty"`
	PaneID    string `json:"pane_id,omitempty"`
	AgentRole string `json:"agent_role,omitempty"`
	PID       int    `json:"pid,omitempty"`
	PiVersion string `json:"pi_version,omitempty"`
	TS        int64  `json:"ts,omitempty"`
}

// Status is an agent → daemon turn-state update.
type Status struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	Task    string `json:"task,omitempty"`
	Tool    string `json:"tool,omitempty"`
	TS      int64  `json:"ts"`
}

// Bye is an agent → daemon clean shutdown notification.
type Bye struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	AgentID string `json:"agent_id"`
	TS      int64  `json:"ts"`
}

// Result is an agent → daemon final-answer notification emitted at agent_end.
type Result struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	AgentID string `json:"agent_id"`
	Text    string `json:"text"`
	TS      int64  `json:"ts"`
}

// Wait is a CLI → daemon request to block until agent_id has a result
// (or transitions to errored). Daemon replies with one AgentUpdate.
type Wait struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	AgentID string `json:"agent_id"`
}

// Focus carries both hook → daemon (pane focused in tmux) and daemon → agent
// (this agent's pane gained/lost focus). Field set varies by direction.
type Focus struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	PaneID  string `json:"pane_id,omitempty"`
	Client  string `json:"client,omitempty"`
	Focused *bool  `json:"focused,omitempty"`
	TS      int64  `json:"ts,omitempty"`
}

// PaneDied is a hook → daemon notification that a tmux pane exited.
type PaneDied struct {
	V      int    `json:"v"`
	Event  string `json:"event"`
	PaneID string `json:"pane_id"`
}

// SessionClosed is a hook → daemon notification that a tmux session ended.
type SessionClosed struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	Session string `json:"session"`
}

// AgentSnapshot is one entry in a Roster or AgentUpdate frame.
type AgentSnapshot struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	Role            string `json:"role,omitempty"`
	Task            string `json:"task,omitempty"`
	Pane            string `json:"pane,omitempty"`
	LastTurnEndedAt int64  `json:"last_turn_ended_at,omitempty"`
	Result          string `json:"result,omitempty"`
}

// Roster is a daemon → sidebar full-state snapshot.
type Roster struct {
	V         int             `json:"v"`
	Event     string          `json:"event"`
	Agents    []AgentSnapshot `json:"agents"`
	LaunchDir string          `json:"launch_dir,omitempty"`
}

// AgentUpdate is a daemon → sidebar single-agent diff.
// Deleted=true signals removal; only Agent.ID is meaningful in that case.
type AgentUpdate struct {
	V       int           `json:"v"`
	Event   string        `json:"event"`
	Agent   AgentSnapshot `json:"agent"`
	Deleted bool          `json:"deleted,omitempty"`
}

// Envelope is a minimal CLI → daemon control frame (shutdown, snapshot).
type Envelope struct {
	V     int    `json:"v"`
	Event string `json:"event"`
}
