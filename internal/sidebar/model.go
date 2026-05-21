package sidebar

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/audibleblink/mure/internal/sock"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Now is overridable for deterministic "elapsed since last turn" tests.
var Now = time.Now

// Model is the Bubble Tea model.
type Model struct {
	agents     []sock.AgentSnapshot
	selected   int
	connected  bool
	tmuxPane   string // $TMUX_PANE
	sessionDir string // tilde-shortened launch dir for the tmux session
	frames     <-chan Frame
	execCmd    func(name string, arg ...string) *exec.Cmd

	width    int
	height   int
	palette  Palette
	dark     bool
	tick     uint64
	expanded bool

	inputMode bool
	inputBuf  string

	focused bool
}

type tickMsg struct{}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// scheduleTick is a var so tests can swap it.
var scheduleTick = func() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func anyWorking(agents []sock.AgentSnapshot) bool {
	for _, a := range agents {
		if a.Status == sock.StatusWorking {
			return true
		}
	}
	return false
}

// NewModel constructs a model. frames is the channel from Client.
// sessionDir, if non-empty, is rendered as a tilde-shortened label under
// the logo (the directory under which `mure up` launched the session).
func NewModel(frames <-chan Frame, tmuxPane, sessionDir string) Model {
	return Model{
		frames:     frames,
		tmuxPane:   tmuxPane,
		sessionDir: tildeShorten(sessionDir),
		execCmd:    exec.Command,
		width:      40,
		height:     0,
		palette:    ActivePalette(),
		dark:       true,
		focused:    true,
	}
}

// tildeShorten replaces a leading $HOME with "~".
func tildeShorten(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// Init starts listening for frames.
func (m Model) Init() tea.Cmd {
	if anyWorking(m.agents) {
		return tea.Batch(waitFrame(m.frames), scheduleTick())
	}
	return waitFrame(m.frames)
}

type frameMsg struct {
	frame Frame
	ok    bool
}

func waitFrame(ch <-chan Frame) tea.Cmd {
	return func() tea.Msg {
		f, ok := <-ch
		return frameMsg{frame: f, ok: ok}
	}
}

// Update handles frames and key events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case frameMsg:
		if !msg.ok {
			return m, tea.Quit
		}
		wasWorking := anyWorking(m.agents)
		m.applyFrame(msg.frame)
		cmd := waitFrame(m.frames)
		if !wasWorking && anyWorking(m.agents) {
			return m, tea.Batch(cmd, scheduleTick())
		}
		return m, cmd
	case tickMsg:
		m.tick++
		if anyWorking(m.agents) {
			return m, scheduleTick()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.FocusMsg:
		m.focused = true
		return m, nil
	case tea.BlurMsg:
		m.focused = false
		return m, nil
	case tea.KeyMsg:
		if m.inputMode {
			return m.handleInputKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) applyFrame(f Frame) {
	switch {
	case f.Roster != nil:
		m.agents = append([]sock.AgentSnapshot(nil), f.Roster.Agents...)
		sortAgents(m.agents)
		m.clampSelection()
		m.connected = true
		if f.Roster.LaunchDir != "" {
			m.sessionDir = tildeShorten(f.Roster.LaunchDir)
		}
	case f.Update != nil:
		if f.Update.Deleted {
			m.remove(f.Update.Agent.ID)
		} else {
			m.upsert(f.Update.Agent)
		}
		m.clampSelection()
	case f.Connect:
		m.connected = true
	default:
		// disconnect
		m.connected = false
	}
}

func (m *Model) remove(id string) {
	for i, x := range m.agents {
		if x.ID == id {
			m.agents = append(m.agents[:i], m.agents[i+1:]...)
			return
		}
	}
}

func (m *Model) upsert(a sock.AgentSnapshot) {
	for i, x := range m.agents {
		if x.ID == a.ID {
			m.agents[i] = a
			return
		}
	}
	m.agents = append(m.agents, a)
	sortAgents(m.agents)
}

func sortAgents(a []sock.AgentSnapshot) {
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
}

func (m *Model) clampSelection() {
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.agents) {
		m.selected = len(m.agents) - 1
	}
	if len(m.agents) == 0 {
		m.selected = 0
	}
}

func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		role := strings.TrimSpace(m.inputBuf)
		if role == "" {
			role = "default"
		}
		_ = m.execCmd("mure", "spawn", role).Start()
		m.inputMode = false
		m.inputBuf = ""
	case tea.KeyEsc, tea.KeyCtrlC:
		m.inputMode = false
		m.inputBuf = ""
	case tea.KeyBackspace, tea.KeyDelete:
		if r := []rune(m.inputBuf); len(r) > 0 {
			m.inputBuf = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.inputBuf += string(msg.Runes)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.selected < len(m.agents)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "enter":
		if m.selected >= 0 && m.selected < len(m.agents) {
			pane := m.agents[m.selected].Pane
			if pane != "" {
				_ = m.execCmd("tmux", "switch-client", "-t", pane).Run()
			}
		}
	case "c", "C":
		m.inputMode = true
		m.inputBuf = ""
	case "i", "I":
		m.expanded = !m.expanded
	case "x", "X":
		if m.selected >= 0 && m.selected < len(m.agents) {
			pane := m.agents[m.selected].Pane
			if pane != "" {
				_ = m.execCmd("tmux", "kill-pane", "-t", pane).Run()
			}
		}
	case "q":
		if m.tmuxPane != "" {
			_ = m.execCmd("tmux", "kill-pane", "-t", m.tmuxPane).Run()
		}
		return m, tea.Quit
	}
	return m, nil
}

// View renders the sidebar (PRD 002 §5, sidebar v2).
// Flat dark panel — no outer border. Logo centered, footer pinned to
// the last line when a height is known.
func (m Model) View() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	inner := w
	bg := m.palette.Background
	bgStyle := lipgloss.NewStyle().Background(bg)
	fg := func(c lipgloss.AdaptiveColor) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c).Background(bg)
	}
	dim := fg(m.palette.Dim)

	pad := func(s string, n int) string {
		v := utf8.RuneCountInString(stripANSI(s))
		if v < n {
			s += bgStyle.Render(strings.Repeat(" ", n-v))
		}
		return s
	}
	center := func(s string, n int) string {
		v := utf8.RuneCountInString(stripANSI(s))
		if v >= n {
			return s
		}
		left := (n - v) / 2
		return bgStyle.Render(strings.Repeat(" ", left)) + s + bgStyle.Render(strings.Repeat(" ", n-v-left))
	}
	blank := bgStyle.Render(strings.Repeat(" ", inner))
	topPad := []string{blank, blank}

	useLogo := inner >= 27
	var header []string
	if useLogo {
		for _, ln := range RenderLogo(Logo, m.palette, m.dark) {
			header = append(header, center(ln, inner))
		}
		header = append(header, blank)
	} else {
		header = []string{center(fg(m.palette.AccentA).Render(runeTrunc(Wordmark, inner)), inner)}
	}

	var dirLine []string
	if m.sessionDir != "" {
		accent := fg(m.palette.AccentA).Italic(true)
		dirLine = []string{pad(center(accent.Render(runeTrunc(m.sessionDir, inner)), inner), inner)}
	}
	countLine := []string{pad(dim.Render(runeTrunc(fmt.Sprintf("%d agents", len(m.agents)), inner)), inner)}
	var disc []string
	if !m.connected {
		disc = []string{pad(fg(m.palette.AccentA).Render(runeTrunc("(disconnected)", inner)), inner)}
	}
	divLine := pad(fg(m.palette.Divider).Render(strings.Repeat("─", inner)), inner)
	topDiv := []string{divLine}
	botDiv := []string{divLine}
	footer := m.buildFooter(inner)
	if m.inputMode {
		footer = m.buildInputLine(inner)
	}

	now := Now()
	// Build both tall and short agent-row variants, pick based on available height.
	shortRows := make([]string, len(m.agents))
	for i, a := range m.agents {
		shortRows[i] = m.renderAgentRow(a, i == m.selected && m.focused, now, inner)
	}
	var tallRows []string
	for i, a := range m.agents {
		tallRows = append(tallRows, m.renderAgentRowTall(a, i == m.selected && m.focused, now, inner)...)
	}

	agentRows := tallRows
	rowsPerAgent := 3
	detailLinesCount := 0
	if m.expanded && m.selected >= 0 && m.selected < len(m.agents) {
		detailLinesCount = 6
	}

	if m.height > 0 {
		fixed := len(topPad) + len(header) + len(countLine) + len(disc) + len(topDiv) + len(botDiv) + len(footer)
		if !m.expanded && fixed+len(tallRows) > m.height {
			agentRows = shortRows
			rowsPerAgent = 1
		}
		_ = detailLinesCount
	}

	var detailLines []string
	if m.expanded && m.selected >= 0 && m.selected < len(m.agents) {
		detail := m.renderAgentDetail(m.agents[m.selected], now, inner)
		for _, ln := range detail {
			detailLines = append(detailLines, pad(ln, inner))
		}
	}

	body := m.layoutDrop(topPad, header, dirLine, countLine, disc, topDiv, agentRows, botDiv, footer, useLogo, inner, blank, rowsPerAgent, detailLines)
	return strings.Join(body, "\n")
}

// buildFooter emits the hint bar, wrapping (key, label) pairs across
// lines to fit `inner`. Keys are bold; labels are italic dim ("ghost").
func (m Model) buildFooter(inner int) []string {
	bg := m.palette.Background
	plain := lipgloss.NewStyle().Background(bg)
	key := lipgloss.NewStyle().Foreground(m.palette.SelectionFG).Background(bg).Bold(true)
	lbl := lipgloss.NewStyle().Foreground(m.palette.Dim).Background(bg).Italic(true)
	pairs := [][2]string{
		{"j/k ↑↓", "select"},
		{"⏎", "focus"},
		{"c", "new"},
		{"i", "info"},
		{"x", "kill"},
		{"q", "quit"},
	}
	padLine := func(s string, n int, w int) string {
		if w < n {
			left := (n - w) / 2
			right := n - w - left
			s = plain.Render(strings.Repeat(" ", left)) + s + plain.Render(strings.Repeat(" ", right))
		}
		return s
	}
	var lines []string
	cur := ""
	curW := 0
	for _, p := range pairs {
		segW := utf8.RuneCountInString(p[0]) + 1 + utf8.RuneCountInString(p[1])
		sepW := 0
		if curW > 0 {
			sepW = 2
		}
		if curW+sepW+segW > inner && curW > 0 {
			lines = append(lines, padLine(cur, inner, curW))
			cur, curW = "", 0
			sepW = 0
		}
		if curW > 0 {
			cur += plain.Render("  ")
			curW += 2
		}
		cur += key.Render(p[0]) + plain.Render(" ") + lbl.Render(p[1])
		curW += segW
	}
	if curW > 0 {
		lines = append(lines, padLine(cur, inner, curW))
	}
	return lines
}

// buildInputLine renders the role-name prompt shown when inputMode is on.
func (m Model) buildInputLine(inner int) []string {
	bg := m.palette.Background
	plain := lipgloss.NewStyle().Background(bg)
	key := lipgloss.NewStyle().Foreground(m.palette.AccentA).Background(bg).Bold(true)
	val := lipgloss.NewStyle().Foreground(m.palette.SelectionFG).Background(bg)
	hint := lipgloss.NewStyle().Foreground(m.palette.Dim).Background(bg).Italic(true)
	prompt := key.Render("spawn:") + plain.Render(" ") + val.Render(m.inputBuf) + key.Render("█") + plain.Render("  ") + hint.Render("⏎ ok  esc cancel")
	w := utf8.RuneCountInString("spawn: " + m.inputBuf + "█  ⏎ ok  esc cancel")
	if w < inner {
		prompt += plain.Render(strings.Repeat(" ", inner-w))
	}
	return []string{prompt}
}

// renderAgentRowTall renders one agent as a 3-line card.
func (m Model) renderAgentRowTall(a sock.AgentSnapshot, selected bool, now time.Time, inner int) []string {
	bg := m.palette.Background
	if selected {
		bg = m.palette.SelectionBG
	}
	plain := lipgloss.NewStyle().Background(bg)
	sty := func(fg lipgloss.AdaptiveColor) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Background(bg)
	}
	padW := func(s string, vis int) string {
		if vis < inner {
			s += plain.Render(strings.Repeat(" ", inner-vis))
		}
		return s
	}

	g := glyph(a.Status, a.LastTurnEndedAt, now)
	if a.Status == sock.StatusWorking {
		g = string(spinnerFrames[int(m.tick%uint64(len(spinnerFrames)))])
	}
	label := a.Status
	name := a.ID
	if a.Role != "" {
		name = a.Role
	}
	bar := plain.Render(" ")
	if selected {
		barCol := lipgloss.AdaptiveColor{Light: "#04a5e5", Dark: "#89dceb"}
		bar = lipgloss.NewStyle().Foreground(barCol).Background(bg).Render("▌")
	}
	elapsed := "—"
	if a.LastTurnEndedAt > 0 {
		age := now.Sub(time.Unix(a.LastTurnEndedAt, 0))
		if age < 0 {
			age = 0
		}
		elapsed = fmt.Sprintf("%dm %02ds", int(age.Minutes()), int(age.Seconds())%60)
	}
	task := a.Task
	if task == "" {
		task = "—"
	}

	statusCol := m.statusColor(a.Status)
	nameFg := m.palette.AccentA
	if selected {
		nameFg = m.palette.SelectionFG
	}

	nameTrunc := runeTrunc(name, inner-4)
	line1 := bar + plain.Render(" ") + sty(statusCol).Render(g) + plain.Render(" ") + sty(nameFg).Bold(true).Render(nameTrunc)
	line1W := 1 + 1 + utf8.RuneCountInString(g) + 1 + utf8.RuneCountInString(nameTrunc)

	taskTrunc := runeTrunc(task, inner-3)
	line2 := bar + plain.Render("  ") + sty(m.palette.Dim).Italic(true).Render(taskTrunc)
	line2W := 3 + utf8.RuneCountInString(taskTrunc)

	meta := label + " · " + elapsed
	if a.Pane != "" {
		meta += " · " + a.Pane
	}
	metaTrunc := runeTrunc(meta, inner-3)
	line3 := bar + plain.Render("  ") + sty(m.palette.Dim).Render(metaTrunc)
	line3W := 3 + utf8.RuneCountInString(metaTrunc)

	return []string{padW(line1, line1W), padW(line2, line2W), padW(line3, line3W)}
}

// renderAgentDetail builds the unfolded info block for the selected agent.
func (m Model) renderAgentDetail(a sock.AgentSnapshot, now time.Time, inner int) []string {
	dim := lipgloss.NewStyle().Foreground(m.palette.Dim).Background(m.palette.Background)
	val := lipgloss.NewStyle().Foreground(m.palette.SelectionFG).Background(m.palette.Background)
	rows := [][2]string{
		{"id", a.ID},
		{"role", a.Role},
		{"status", a.Status},
		{"task", a.Task},
		{"pane", a.Pane},
	}
	last := "—"
	if a.LastTurnEndedAt > 0 {
		age := now.Sub(time.Unix(a.LastTurnEndedAt, 0))
		if age < 0 {
			age = 0
		}
		last = fmt.Sprintf("%dm %02ds ago", int(age.Minutes()), int(age.Seconds())%60)
	}
	rows = append(rows, [2]string{"last", last})
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		v := r[1]
		if v == "" {
			v = "—"
		}
		line := "   " + dim.Render(fmt.Sprintf("%-7s", r[0])) + val.Render(runeTrunc(v, inner-10))
		out = append(out, line)
	}
	return out
}

func insertDetails(agentRows, detailLines []string, insertAt int) []string {
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(agentRows) {
		insertAt = len(agentRows)
	}
	rows := make([]string, 0, len(agentRows)+len(detailLines))
	rows = append(rows, agentRows[:insertAt]...)
	rows = append(rows, detailLines...)
	rows = append(rows, agentRows[insertAt:]...)
	return rows
}

// layoutDrop applies the height-driven drop policy (PRD 002 §7.2) and
// pins the footer to the last line by inserting background-filled
// blanks between the agent rows and the bottom divider when the
// terminal height exceeds the content.
func (m Model) layoutDrop(topPad, header, dirLine, countLine, disc, topDiv, agentRows, botDiv, footer []string, useLogo bool, inner int, blank string, rowsPerAgent int, detailLines []string) []string {
	var filler []string
	if m.height > 0 {
		innerH := m.height
		if innerH < 0 {
			innerH = 0
		}
		total := len(topPad) + len(header) + len(dirLine) + len(countLine) + len(disc) + len(topDiv) + len(agentRows) + len(detailLines) + len(botDiv) + len(footer)
		if total > innerH {
			total -= len(topPad)
			topPad = nil
		}
		if total > innerH {
			total -= len(footer)
			footer = nil
		}
		if total > innerH {
			total -= len(botDiv)
			botDiv = nil
		}
		if total > innerH && useLogo {
			total -= len(header)
			header = []string{lipgloss.NewStyle().Foreground(m.palette.AccentA).Background(m.palette.Background).Render(runeTrunc(Wordmark, inner))}
			total += len(header)
			useLogo = false
		}
		if total > innerH {
			total -= len(header)
			header = nil
		}
		if total > innerH {
			total -= len(countLine) + len(disc)
			countLine = nil
			disc = nil
		}
		if total > innerH {
			total -= len(dirLine)
			dirLine = nil
		}
		if total > innerH {
			target := innerH - (len(topPad) + len(header) + len(dirLine) + len(countLine) + len(disc) + len(topDiv) + len(botDiv) + len(footer) + len(detailLines))
			if target < 0 {
				target = 0
			}
			var startAgent int
			agentRows, startAgent = clipAgents(agentRows, m.selected, target, rowsPerAgent)
			
			if len(detailLines) > 0 {
				insertAt := (m.selected - startAgent + 1) * rowsPerAgent
				agentRows = insertDetails(agentRows, detailLines, insertAt)
			}
			
			total = len(topPad) + len(header) + len(dirLine) + len(countLine) + len(disc) + len(topDiv) + len(agentRows) + len(botDiv) + len(footer)
		} else if len(detailLines) > 0 {
			insertAt := (m.selected + 1) * rowsPerAgent
			agentRows = insertDetails(agentRows, detailLines, insertAt)
		}
		if total < innerH {
			filler = make([]string, innerH-total)
			for i := range filler {
				filler[i] = blank
			}
		}
	} else if len(detailLines) > 0 {
		insertAt := (m.selected + 1) * rowsPerAgent
		agentRows = insertDetails(agentRows, detailLines, insertAt)
	}
	out := make([]string, 0, len(topPad)+len(header)+len(dirLine)+len(countLine)+len(disc)+len(topDiv)+len(agentRows)+len(filler)+len(botDiv)+len(footer))
	out = append(out, topPad...)
	out = append(out, header...)
	out = append(out, dirLine...)
	out = append(out, countLine...)
	out = append(out, disc...)
	out = append(out, topDiv...)
	out = append(out, agentRows...)
	out = append(out, filler...)
	out = append(out, botDiv...)
	out = append(out, footer...)
	return out
}

// clipAgents returns a window of `target` lines into rows, shifted so
// that the selected agent stays visible. rowsPerAgent>1 means rows are
// grouped in fixed-size blocks per agent; clip then snaps to whole-agent
// boundaries.
func clipAgents(rows []string, selected, target, rowsPerAgent int) ([]string, int) {
	if target <= 0 || len(rows) == 0 {
		return nil, 0
	}
	if rowsPerAgent <= 1 {
		if target >= len(rows) {
			return rows, 0
		}
		start := 0
		if selected >= target {
			start = selected - target + 1
		}
		if start+target > len(rows) {
			start = len(rows) - target
		}
		if start < 0 {
			start = 0
		}
		return rows[start : start+target], start
	}
	nAgents := len(rows) / rowsPerAgent
	agentTarget := target / rowsPerAgent
	if agentTarget <= 0 {
		return nil, 0
	}
	if agentTarget >= nAgents {
		return rows[:nAgents*rowsPerAgent], 0
	}
	start := 0
	if selected >= agentTarget {
		start = selected - agentTarget + 1
	}
	if start+agentTarget > nAgents {
		start = nAgents - agentTarget
	}
	if start < 0 {
		start = 0
	}
	return rows[start*rowsPerAgent : (start+agentTarget)*rowsPerAgent], start
}

// glyph returns the status glyph (PRD §9).
func glyph(status string, lastEndedAt int64, now time.Time) string {
	switch status {
	case sock.StatusWorking:
		return "●"
	case sock.StatusBlocked:
		return "◐"
	case sock.StatusIdle:
		if lastEndedAt > 0 && now.Sub(time.Unix(lastEndedAt, 0)) < 5*time.Minute {
			return "✓"
		}
		return "○"
	}
	return "?"
}

// renderAgentRow renders one agent line, styled and padded to inner width.
func (m Model) renderAgentRow(a sock.AgentSnapshot, selected bool, now time.Time, inner int) string {
	g := glyph(a.Status, a.LastTurnEndedAt, now)
	if a.Status == sock.StatusWorking {
		g = string(spinnerFrames[int(m.tick%uint64(len(spinnerFrames)))])
	}
	label := a.Status
	elapsed := "—"
	var age time.Duration
	if a.LastTurnEndedAt > 0 {
		age = now.Sub(time.Unix(a.LastTurnEndedAt, 0))
		if age < 0 {
			age = 0
		}
		elapsed = fmt.Sprintf("%d:%02d", int(age.Minutes()), int(age.Seconds())%60)
	}
	name := a.ID
	if a.Role != "" {
		name = a.Role
	}
	marker := " "
	if selected {
		marker = "▸"
	}

	bg := m.palette.Background
	if selected {
		plain := fmt.Sprintf("%s %s %-10s %-8s %s", marker, g, name, label, elapsed)
		rl := utf8.RuneCountInString(plain)
		if rl < inner {
			plain += strings.Repeat(" ", inner-rl)
		} else if rl > inner {
			plain = runeTrunc(plain, inner)
		}
		return lipgloss.NewStyle().
			Foreground(m.palette.SelectionFG).
			Background(m.palette.SelectionBG).
			Render(plain)
	}

	statusCol := m.statusColor(a.Status)
	elapsedCol := m.palette.Dim
	if age > 5*time.Minute {
		elapsedCol = m.palette.AccentA
	}

	namePad := fmt.Sprintf("%-10s", name)
	labelPad := fmt.Sprintf("%-8s", label)
	space := lipgloss.NewStyle().Background(bg).Render(" ")
	markerS := lipgloss.NewStyle().Foreground(m.palette.AccentA).Background(bg).Render(marker)
	glyphS := lipgloss.NewStyle().Foreground(statusCol).Background(bg).Render(g)
	nameS := lipgloss.NewStyle().Foreground(m.palette.SelectionFG).Background(bg).Render(namePad)
	labelS := lipgloss.NewStyle().Foreground(statusCol).Background(bg).Render(labelPad)
	elapsedS := lipgloss.NewStyle().Foreground(elapsedCol).Background(bg).Render(elapsed)

	row := markerS + space + glyphS + space + nameS + space + labelS + space + elapsedS
	visLen := utf8.RuneCountInString(marker) + 1 + utf8.RuneCountInString(g) + 1 +
		utf8.RuneCountInString(namePad) + 1 + utf8.RuneCountInString(labelPad) + 1 +
		utf8.RuneCountInString(elapsed)
	if visLen < inner {
		row += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", inner-visLen))
	}
	return row
}

func (m Model) statusColor(s string) lipgloss.AdaptiveColor {
	switch s {
	case sock.StatusWorking:
		return m.palette.Working
	case sock.StatusBlocked:
		return m.palette.Blocked
	case sock.StatusIdle:
		return m.palette.Idle
	}
	return m.palette.Dim
}

func runeTrunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// TmuxPane returns the configured TMUX_PANE (for tests).
func (m Model) TmuxPane() string { return m.tmuxPane }

// Run wires the client + model with bubbletea.Program and runs until ctx ends.
func Run(ctx context.Context) error {
	sockPath := os.Getenv("MURE_SOCKET")
	if sockPath == "" {
		return fmt.Errorf("MURE_SOCKET not set")
	}
	c := NewClient(sockPath)
	go c.Run(ctx)
	m := NewModel(c.Frames, os.Getenv("TMUX_PANE"), "")
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithReportFocus())
	_, err := p.Run()
	return err
}
