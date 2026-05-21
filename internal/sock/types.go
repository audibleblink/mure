// Package sock defines the daemon wire protocol (PRD §12).
package sock

// ProtocolVersion is the v1 wire version. Daemons reject any other value.
const ProtocolVersion = 1

// MaxFrameSize is the maximum NDJSON line length in bytes (PRD §12).
const MaxFrameSize = 64 * 1024

// Status vocabulary.
const (
	StatusIdle    = "idle"
	StatusWorking = "working"
	StatusBlocked = "blocked"
)

// ValidStatus reports whether s is a known status string.
func ValidStatus(s string) bool {
	switch s {
	case StatusIdle, StatusWorking, StatusBlocked:
		return true
	}
	return false
}

// Connection roles (first-frame hello.role).
const (
	RoleAgent   = "agent"
	RoleSidebar = "sidebar"
	RoleCLI     = "cli"
)

// Hello is the first frame sent by every connection.
// Agent connections populate AgentID/PaneID/PID/PiVersion/TS; cli/sidebar
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

// Wait is a CLI → daemon request to block until agent_id has a result.
// Daemon replies with one AgentUpdate.
type Wait struct {
	V       int    `json:"v"`
	Event   string `json:"event"`
	AgentID string `json:"agent_id"`
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
