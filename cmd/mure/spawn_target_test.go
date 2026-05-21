package main

import (
	"reflect"
	"strings"
	"testing"
)

type recRunner struct {
	responses map[string]string
	errs      map[string]error
	calls     [][]string
}

func newRecRunner() *recRunner {
	return &recRunner{responses: map[string]string{}, errs: map[string]error{}}
}

func (r *recRunner) run(args ...string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if err, ok := r.errs[key]; ok {
		return "", err
	}
	return r.responses[key], nil
}

func TestPickSpawnTarget_Template_SplitHorizontal(t *testing.T) {
	r := newRecRunner()
	plan, err := pickSpawnTarget(r.run, "split-window -h", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"split-window", "-h", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
	if plan.PostCreate != nil {
		t.Errorf("PostCreate should be nil")
	}
	if len(r.calls) != 0 {
		t.Errorf("expected no runner calls, got %v", r.calls)
	}
}

func TestPickSpawnTarget_Template_NewWindow(t *testing.T) {
	r := newRecRunner()
	plan, err := pickSpawnTarget(r.run, "new-window", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"new-window", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
}

func TestPickSpawnTarget_Template_ArbitraryFlags(t *testing.T) {
	r := newRecRunner()
	plan, err := pickSpawnTarget(r.run, "split-window -h -f -l 40%", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"split-window", "-h", "-f", "-l", "40%", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
}

func TestPickSpawnTarget_EmptyDefaultsToSubagentsWindow(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	r := newRecRunner()
	r.responses["display-message -p #{session_id}"] = "$1"
	r.responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = ""
	plan, err := pickSpawnTarget(r.run, "", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"new-window", "-d", "-t", "$1", "-n", "subagents", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
	if plan.PostCreate == nil {
		t.Errorf("PostCreate should not be nil")
	}
}

func TestPickSpawnTarget_SubagentsWindow_Missing(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	r := newRecRunner()
	r.responses["display-message -p #{session_id}"] = "$1"
	r.responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = ""
	plan, err := pickSpawnTarget(r.run, "subagents-window", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"new-window", "-d", "-t", "$1", "-n", "subagents", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
	if plan.PostCreate == nil {
		t.Fatalf("PostCreate should not be nil")
	}
	r.responses["display-message -p -t %42 #{window_id}"] = "@7"
	r.responses["set-option -w -t @7 @mure-subagents-window 1"] = ""
	prior := len(r.calls)
	if err := plan.PostCreate("%42"); err != nil {
		t.Fatalf("PostCreate error: %v", err)
	}
	post := r.calls[prior:]
	if len(post) != 2 {
		t.Fatalf("expected 2 follow-up calls, got %v", post)
	}
	want0 := []string{"display-message", "-p", "-t", "%42", "#{window_id}"}
	want1 := []string{"set-option", "-w", "-t", "@7", "@mure-subagents-window", "1"}
	if !reflect.DeepEqual(post[0], want0) {
		t.Errorf("post[0] = %v, want %v", post[0], want0)
	}
	if !reflect.DeepEqual(post[1], want1) {
		t.Errorf("post[1] = %v, want %v", post[1], want1)
	}
}

func TestPickSpawnTarget_SubagentsWindow_MarkerPresent(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	r := newRecRunner()
	r.responses["display-message -p #{session_id}"] = "$1"
	r.responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = "@3 misc \n@7 subagents 1\n@9 other "
	plan, err := pickSpawnTarget(r.run, "subagents-window", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"split-window", "-h", "-t", "@7", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
	if plan.PostCreate == nil {
		t.Errorf("PostCreate should rebalance layout")
	}
}

func TestPickSpawnTarget_SubagentsWindow_NameOnly(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	r := newRecRunner()
	r.responses["display-message -p #{session_id}"] = "$1"
	r.responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = "@5 subagents"
	plan, err := pickSpawnTarget(r.run, "subagents-window", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"split-window", "-h", "-t", "@5", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
}

func TestPickSpawnTarget_SubagentsWindow_MarkerBeatsName(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	r := newRecRunner()
	r.responses["display-message -p #{session_id}"] = "$1"
	r.responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = "@4 subagents \n@8 agents 1"
	plan, err := pickSpawnTarget(r.run, "subagents-window", "PL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"split-window", "-h", "-t", "@8", "-P", "-F", "#{pane_id}", "PL"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Errorf("Argv = %v, want %v", plan.Argv, want)
	}
}

func TestPickSpawnTarget_SessionLookup_TmuxPaneSet(t *testing.T) {
	t.Setenv("TMUX_PANE", "%99")
	r := newRecRunner()
	r.responses["display-message -p -t %99 #{session_id}"] = "$2"
	r.responses["list-windows -t $2 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = ""
	if _, err := pickSpawnTarget(r.run, "subagents-window", "PL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want0 := []string{"display-message", "-p", "-t", "%99", "#{session_id}"}
	if !reflect.DeepEqual(r.calls[0], want0) {
		t.Errorf("calls[0] = %v, want %v", r.calls[0], want0)
	}
}

func TestPickSpawnTarget_SessionLookup_TmuxPaneUnset(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	r := newRecRunner()
	r.responses["display-message -p #{session_id}"] = "$1"
	r.responses["list-windows -t $1 -F #{window_id} #{window_name} #{@mure-subagents-window}"] = ""
	if _, err := pickSpawnTarget(r.run, "subagents-window", "PL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want0 := []string{"display-message", "-p", "#{session_id}"}
	if !reflect.DeepEqual(r.calls[0], want0) {
		t.Errorf("calls[0] = %v, want %v", r.calls[0], want0)
	}
}
