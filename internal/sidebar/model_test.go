package sidebar

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/audibleblink/mure/internal/sock"
	tea "github.com/charmbracelet/bubbletea"
)

var update = flag.Bool("update-golden", false, "rewrite golden files")

// fixed "now" so elapsed rendering is deterministic.
var testNow = time.Unix(1_700_000_300, 0)

func init() { Now = func() time.Time { return testNow } }

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.txt")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update-golden to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

func newWithFrames(frames <-chan Frame) Model {
	return NewModel(frames, "%9", "")
}

// resize fakes a WindowSizeMsg into the model.
func resize(m Model, w, h int) Model {
	res, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return res.(Model)
}

// strippedLines returns the rendered view, ANSI-stripped, split by newline.
func strippedLines(m Model) []string {
	return strings.Split(stripANSI(m.View()), "\n")
}

func TestView_DefaultWidth(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "agent-idle", Status: sock.StatusIdle},
		{ID: "agent-work", Status: sock.StatusWorking},
		{ID: "agent-blok", Status: sock.StatusBlocked},
	}}})
	assertGolden(t, "default_width", stripANSI(m.View()))
}

func TestView_NarrowWidthDropsLogo(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a1", Status: sock.StatusIdle},
	}}})
	m = resize(m, 20, 0)

	lines := strippedLines(m)
	for i, ln := range lines {
		if n := len([]rune(ln)); n > 20 {
			t.Errorf("line %d width %d > 20: %q", i, n, ln)
		}
	}
	if len(lines) < 1 {
		t.Fatalf("not enough lines: %v", lines)
	}
	// First content line is the centered wordmark. Width=20, "mure"=4 → centered.
	// wordmark sits after the 2-line top pad.
	if !strings.Contains(lines[2], Wordmark) {
		t.Errorf("wordmark line %q does not contain wordmark", lines[2])
	}
	if n := len([]rune(lines[2])); n != 20 {
		t.Errorf("wordmark line width %d want 20: %q", n, lines[2])
	}
}

func TestView_SmallHeightDropsRegions(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a1", Status: sock.StatusIdle},
		{ID: "a2", Status: sock.StatusWorking},
	}}})

	footer := "j/k"
	logoFirst := "███"

	containsAny := func(lines []string, s string) bool {
		for _, ln := range lines {
			if strings.Contains(ln, s) {
				return true
			}
		}
		return false
	}

	// Full layout (no height constraint) — everything present.
	full := strippedLines(resize(m, 36, 0))
	if !containsAny(full, footer) {
		t.Fatalf("baseline: footer missing")
	}
	if !containsAny(full, logoFirst) {
		t.Fatalf("baseline: logo missing")
	}

	// Step 1: shrink by 1 — tall rows collapse to short; footer still present.
	step1 := strippedLines(resize(m, 36, len(full)-1))
	if !containsAny(step1, footer) {
		t.Errorf("step1: footer dropped too early, got %v", step1)
	}
	// short rows contain both name and status label on the same line.
	shortRowFound := false
	for _, ln := range step1 {
		if strings.Contains(ln, "a1") && strings.Contains(ln, "idle") {
			shortRowFound = true
			break
		}
	}
	if !shortRowFound {
		t.Errorf("step1: expected tall→short collapse, got %v", step1)
	}

	// Step 2: tight enough to drop topPad + footer; logo still present.
	step2 := strippedLines(resize(m, 36, 10))
	if !containsAny(step2, logoFirst) {
		t.Errorf("step2: logo dropped too early, got %v", step2)
	}
	if containsAny(step2, footer) {
		t.Errorf("step2: footer should have been dropped, got %v", step2)
	}

	// Step 3: logo downgraded to wordmark.
	step3 := strippedLines(resize(m, 36, 6))
	if containsAny(step3, logoFirst) {
		t.Errorf("step3: logo should be downgraded by now, got %v", step3)
	}
	if !containsAny(step3, "mure") {
		t.Errorf("step3: expected wordmark fallback, got %v", step3)
	}

	// Step 4: tiny — wordmark and count line drop, agent rows survive.
	step4 := strippedLines(resize(m, 36, 4))
	if containsAny(step4, "mure") {
		t.Errorf("step4: wordmark should have been dropped, got %v", step4)
	}
	if !containsAny(step4, "a1") && !containsAny(step4, "a2") {
		t.Errorf("step4: expected at least one agent row, got %v", step4)
	}
}

func TestView_SelectedRowStaysVisible(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	var ags []sock.AgentSnapshot
	for i := 0; i < 12; i++ {
		ags = append(ags, sock.AgentSnapshot{
			ID:     "agent-" + string(rune('a'+i)),
			Status: sock.StatusIdle,
		})
	}
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: ags}})
	for i := 0; i < 10; i++ {
		res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = res.(Model)
	}
	if m.selected != 10 {
		t.Fatalf("selected=%d want 10", m.selected)
	}
	// Tight height that can only fit a few agent rows.
	m = resize(m, 36, 6)
	out := stripANSI(m.View())
	if !strings.Contains(out, ags[10].ID) {
		t.Errorf("selected agent %s not visible in:\n%s", ags[10].ID, out)
	}
}

func TestView_SelectionStyling(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "alpha", Status: sock.StatusIdle},
		{ID: "beta", Status: sock.StatusWorking},
	}}})
	// selected=0 by default
	lines := strippedLines(m)
	var rowText string
	for _, ln := range lines {
		if strings.Contains(ln, "▌") {
			rowText = ln
			break
		}
	}
	if rowText == "" {
		t.Fatalf("no row containing ▌ found in:\n%s", strings.Join(lines, "\n"))
	}
	inner := 40
	if got := len([]rune(rowText)); got != inner {
		t.Errorf("selected row width %d want %d (row=%q)", got, inner, rowText)
	}
}

func TestKeybindingsUnchanged(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "agent-1"}, {ID: "agent-2"}, {ID: "agent-3"},
	}}})
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = res.(Model)
	if m.selected != 1 {
		t.Fatalf("j: expected selected=1, got %d", m.selected)
	}
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = res.(Model)
	if m.selected != 2 {
		t.Fatalf("down: expected selected=2, got %d", m.selected)
	}
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = res.(Model)
	if m.selected != 2 {
		t.Fatalf("down: should clamp to 2, got %d", m.selected)
	}
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = res.(Model)
	if m.selected != 1 {
		t.Fatalf("k: expected selected=1, got %d", m.selected)
	}
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = res.(Model)
	if m.selected != 0 {
		t.Fatalf("up: expected selected=0, got %d", m.selected)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("q: expected tea.Quit cmd")
	}
}

func TestView_AgentUpdate(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "agent-1", Status: sock.StatusIdle, Pane: "%1"},
	}}})
	m.applyFrame(Frame{Update: &sock.AgentUpdate{V: 1, Event: "agent_update",
		Agent: sock.AgentSnapshot{ID: "agent-1", Status: sock.StatusWorking, Task: "x", Pane: "%1"},
	}})
	if got := m.agents[0].Status; got != sock.StatusWorking {
		t.Fatalf("update not applied: %s", got)
	}
}

func TestKey_EnterShellsOutTmuxSelectPane(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "agent-1", Pane: "%42"},
	}}})
	var gotName string
	var gotArgs []string
	m.execCmd = func(name string, arg ...string) *exec.Cmd {
		gotName = name
		gotArgs = arg
		return exec.Command("true")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if gotName != "tmux" || len(gotArgs) != 3 || gotArgs[0] != "switch-client" || gotArgs[2] != "%42" {
		t.Fatalf("bad exec: %s %v", gotName, gotArgs)
	}
}

// withTickHook swaps scheduleTick with a recorder; returns a *bool that
// flips to true if any tick is scheduled, and a restore func.
func withTickHook(t *testing.T) (*bool, func()) {
	t.Helper()
	prev := scheduleTick
	called := false
	scheduleTick = func() tea.Cmd {
		called = true
		return nil
	}
	return &called, func() { scheduleTick = prev }
}

func TestSpinner_NoTicksWhenNoWorking(t *testing.T) {
	called, restore := withTickHook(t)
	defer restore()
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusIdle},
		{ID: "b", Status: sock.StatusBlocked},

	}}})
	_ = m.Init()
	if *called {
		t.Fatalf("Init scheduled a tick with no working agents")
	}
	// Apply a frame that does not introduce a working agent.
	_, _ = m.Update(frameMsg{ok: true, frame: Frame{Update: &sock.AgentUpdate{V: 1, Event: "agent_update",
		Agent: sock.AgentSnapshot{ID: "a", Status: sock.StatusIdle},
	}}})
	if *called {
		t.Fatalf("non-working frame scheduled a tick")
	}
}

func TestSpinner_TickArmedOnTransition(t *testing.T) {
	called, restore := withTickHook(t)
	defer restore()
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusIdle},
	}}})
	if *called {
		t.Fatalf("unexpected tick before transition")
	}
	_, _ = m.Update(frameMsg{ok: true, frame: Frame{Update: &sock.AgentUpdate{V: 1, Event: "agent_update",
		Agent: sock.AgentSnapshot{ID: "a", Status: sock.StatusWorking},
	}}})
	if !*called {
		t.Fatalf("transition to working did not arm a tick")
	}
}

func TestSpinner_FrameAdvances(t *testing.T) {
	_, restore := withTickHook(t)
	defer restore()
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusWorking},
	}}})
	out0 := stripANSI(m.View())
	if !strings.ContainsRune(out0, spinnerFrames[0]) {
		t.Fatalf("expected frame 0 %q in output:\n%s", string(spinnerFrames[0]), out0)
	}
	res, _ := m.Update(tickMsg{})
	m = res.(Model)
	out1 := stripANSI(m.View())
	if !strings.ContainsRune(out1, spinnerFrames[1]) {
		t.Fatalf("expected frame 1 %q in output:\n%s", string(spinnerFrames[1]), out1)
	}
	if out0 == out1 {
		t.Fatalf("spinner did not advance between ticks")
	}
}

func TestSpinner_StopsWhenWorkAtComplete(t *testing.T) {
	called, restore := withTickHook(t)
	defer restore()
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusWorking},
	}}})
	// Frame transitions agent to idle.
	res, _ := m.Update(frameMsg{ok: true, frame: Frame{Update: &sock.AgentUpdate{V: 1, Event: "agent_update",
		Agent: sock.AgentSnapshot{ID: "a", Status: sock.StatusIdle},
	}}})
	m = res.(Model)
	*called = false
	_, cmd := m.Update(tickMsg{})
	if cmd != nil {
		t.Fatalf("expected nil cmd after work complete, got %T", cmd)
	}
	if *called {
		t.Fatalf("unexpected re-arm after work complete")
	}
}

// firstSGR returns the first ANSI SGR escape sequence in s, or "" if none.
func firstSGR(s string) string {
	return ansiRE.FindString(s)
}

func TestView_DisconnectedIndicatorColor(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusIdle},
	}}})
	m.applyFrame(Frame{})
	// Find the rendered line containing the disconnected indicator (raw, ANSI intact).
	var discLine string
	for _, ln := range strings.Split(m.View(), "\n") {
		if strings.Contains(stripANSI(ln), "(disconnected)") {
			discLine = ln
			break
		}
	}
	if discLine == "" {
		t.Fatalf("(disconnected) line not found")
	}
	wantHex := m.palette.AccentA.Dark
	if !m.dark {
		wantHex = m.palette.AccentA.Light
	}
	wantR, wantG, wantB := parseHex(wantHex)
	wantFrag := fmt.Sprintf("38;2;%d;%d;%d", wantR, wantG, wantB)
	if !strings.Contains(discLine, wantFrag) {
		t.Errorf("disconnected line missing AccentA fg %q in %q", wantFrag, discLine)
	}
}

func TestView_DisconnectedLinePresent(t *testing.T) {
	conn := newWithFrames(nil)
	conn.applyFrame(Frame{Connect: true})
	conn.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusIdle},
	}}})
	connLines := strippedLines(conn)

	dis := newWithFrames(nil)
	dis.applyFrame(Frame{Connect: true})
	dis.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusIdle},
	}}})
	dis.applyFrame(Frame{})
	if dis.connected {
		t.Fatalf("expected connected=false after empty frame")
	}
	disLines := strippedLines(dis)

	// Logo lines (after 2-line top pad) must be byte-identical.
	for i := 2; i <= 4; i++ {
		if connLines[i] != disLines[i] {
			t.Errorf("logo line %d changed on disconnect:\nconn=%q\ndis =%q", i, connLines[i], disLines[i])
		}
	}

	// Find the disconnected line and the top divider; disconnected must precede.
	discIdx, divIdx := -1, -1
	for i, ln := range disLines {
		if strings.Contains(ln, "(disconnected)") && discIdx < 0 {
			discIdx = i
		}
		if discIdx >= 0 && divIdx < 0 && i > discIdx && strings.Contains(ln, "───") {
			divIdx = i
		}
	}
	if discIdx < 0 {
		t.Fatalf("(disconnected) line not found in:\n%s", strings.Join(disLines, "\n"))
	}
	if divIdx < 0 || divIdx <= discIdx {
		t.Fatalf("top divider must follow (disconnected): discIdx=%d divIdx=%d", discIdx, divIdx)
	}
}

func TestView_LogoCentered(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m = resize(m, 40, 0)
	lines := strippedLines(m)
	if len(lines) < 1 {
		t.Fatalf("empty render")
	}
	// logo first line is 26 visible runes; in width 40 expect 7 leading spaces.
	// (first 2 lines are top padding)
	logoLine := lines[2]
	trimmed := strings.TrimLeft(logoLine, " ")
	left := len(logoLine) - len(trimmed)
	want := (40 - 26) / 2
	if left != want {
		t.Errorf("logo left padding %d want %d (line=%q)", left, want, logoLine)
	}
}

func TestView_FullHeightPinsFooter(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Status: sock.StatusIdle},
	}}})
	m = resize(m, 36, 30)
	lines := strippedLines(m)
	if len(lines) != 30 {
		t.Fatalf("render height %d want 30", len(lines))
	}
	if !strings.Contains(lines[29], "q quit") {
		t.Errorf("footer last line wrong: %q", lines[29])
	}
	if !strings.Contains(lines[28], "i info") && !strings.Contains(lines[28], "c new") {
		t.Errorf("footer first line wrong: %q", lines[28])
	}
}

func TestKey_IToggleInfo(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Connect: true})
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "alpha", Role: "builder", Status: sock.StatusIdle, Task: "do thing", Pane: "%7"},
	}}})
	before := stripANSI(m.View())
	// id ("alpha") is only rendered by the info panel; tall rows show role.
	if strings.Contains(before, "alpha") {
		t.Fatalf("id should not be visible before toggle")
	}
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = res.(Model)
	after := stripANSI(m.View())
	for _, want := range []string{"alpha", "do thing", "builder", "%7"} {
		if !strings.Contains(after, want) {
			t.Errorf("expanded view missing %q in:\n%s", want, after)
		}
	}
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = res.(Model)
	again := stripANSI(m.View())
	if strings.Contains(again, "alpha") {
		t.Errorf("second toggle did not collapse info panel")
	}
}

func TestKey_XKillsSelectedAgentPane(t *testing.T) {
	m := newWithFrames(nil)
	m.applyFrame(Frame{Roster: &sock.Roster{V: 1, Event: "roster", Agents: []sock.AgentSnapshot{
		{ID: "a", Pane: "%11"},
		{ID: "b", Pane: "%22"},
	}}})
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = res.(Model)
	var gotName string
	var gotArgs []string
	m.execCmd = func(name string, arg ...string) *exec.Cmd {
		gotName = name
		gotArgs = arg
		return exec.Command("true")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if gotName != "tmux" || len(gotArgs) != 3 || gotArgs[0] != "kill-pane" || gotArgs[2] != "%22" {
		t.Fatalf("bad exec: %s %v", gotName, gotArgs)
	}
	if cmd != nil {
		t.Errorf("x should not return a tea cmd, got %T", cmd)
	}
}

func TestThemePicker_OpenCycleCommit(t *testing.T) {
	m := newWithFrames(nil)
	start := m.themeIdx

	// `t` opens picker.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = res.(Model)
	if !m.themePicker {
		t.Fatal("expected themePicker true after 't'")
	}

	// `j` moves down and live-applies the next palette.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = res.(Model)
	if m.themeIdx != start+1 {
		t.Fatalf("themeIdx after j: got %d want %d", m.themeIdx, start+1)
	}
	if m.palette.Background != Themes[m.themeIdx].Palette.Background {
		t.Fatal("palette did not live-update on cycle")
	}

	// Enter commits and closes.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if m.themePicker {
		t.Fatal("expected themePicker false after enter")
	}
	if m.themeIdx != start+1 {
		t.Fatalf("themeIdx after commit: got %d want %d", m.themeIdx, start+1)
	}
}

func TestThemePicker_EscRestores(t *testing.T) {
	m := newWithFrames(nil)
	start := m.themeIdx

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = res.(Model)

	if m.themePicker {
		t.Fatal("expected themePicker false after esc")
	}
	if m.themeIdx != start {
		t.Fatalf("themeIdx after esc: got %d want %d", m.themeIdx, start)
	}
	if m.palette.Background != Themes[start].Palette.Background {
		t.Fatal("palette not restored after esc")
	}
}

func TestKey_QKillsPane(t *testing.T) {
	m := NewModel(nil, "%9", "")
	var gotName string
	var gotArgs []string
	m.execCmd = func(name string, arg ...string) *exec.Cmd {
		gotName = name
		gotArgs = arg
		return exec.Command("true")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if gotName != "tmux" || gotArgs[0] != "kill-pane" || gotArgs[2] != "%9" {
		t.Fatalf("bad exec: %s %v", gotName, gotArgs)
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit cmd")
	}
}
