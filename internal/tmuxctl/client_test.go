package tmuxctl

import (
	"bufio"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// newPipes wires an in-process "tmux" peer to the client: returns the
// client's stdin/stdout endpoints plus the matching reader (for inspecting
// the commands the client wrote) and writer (for emitting tmux output).
func newPipes() (stdin io.WriteCloser, stdout io.ReadCloser, tmuxRead *bufio.Reader, tmuxWrite *io.PipeWriter) {
	// client.stdin: client writes commands here; tmux side reads them.
	inR, inW := io.Pipe()
	// client.stdout: tmux side writes events/replies here; client reads them.
	outR, outW := io.Pipe()
	return inW, outR, bufio.NewReader(inR), outW
}

func TestRealClient_ReplyCorrelation(t *testing.T) {
	stdin, stdout, tmuxIn, tmuxOut := newPipes()
	c := newClient(nil, stdin, stdout)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Drive the fake tmux: read commands, emit canned replies.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait for the first command, then reply.
		cmd1, _ := tmuxIn.ReadString('\n')
		if strings.TrimSpace(cmd1) != "list-windows" {
			t.Errorf("cmd1 = %q, want list-windows", cmd1)
		}
		_, _ = io.WriteString(tmuxOut, "%begin 1 1 0\nwin1\nwin2\n%end 1 1 0\n")
		cmd2, _ := tmuxIn.ReadString('\n')
		if strings.TrimSpace(cmd2) != "display-message" {
			t.Errorf("cmd2 = %q, want display-message", cmd2)
		}
		_, _ = io.WriteString(tmuxOut, "%begin 2 2 0\nhello\n%end 2 2 0\n")
	}()

	out1, err := c.Run(ctx, "list-windows")
	if err != nil {
		t.Fatalf("Run1 err: %v", err)
	}
	if out1 != "win1\nwin2" {
		t.Fatalf("out1 = %q", out1)
	}
	out2, err := c.Run(ctx, "display-message")
	if err != nil {
		t.Fatalf("Run2 err: %v", err)
	}
	if out2 != "hello" {
		t.Fatalf("out2 = %q", out2)
	}
	wg.Wait()
}

func TestRealClient_ErrorReply(t *testing.T) {
	stdin, stdout, tmuxIn, tmuxOut := newPipes()
	c := newClient(nil, stdin, stdout)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_, _ = tmuxIn.ReadString('\n')
		_, _ = io.WriteString(tmuxOut, "%begin 1 1 0\nunknown command: foo\n%error 1 1 0\n")
	}()

	_, err := c.Run(ctx, "foo")
	if err == nil {
		t.Fatal("expected error from error reply")
	}
	if !strings.Contains(err.Error(), "unknown command: foo") {
		t.Fatalf("err = %v, missing payload", err)
	}
}

func TestRealClient_AsyncEventsRoutedToChannel(t *testing.T) {
	stdin, stdout, tmuxIn, tmuxOut := newPipes()
	c := newClient(nil, stdin, stdout)
	defer c.Close()

	// Push an event before any command.
	_, _ = io.WriteString(tmuxOut, "%pane-died %41\n")

	select {
	case ev := <-c.Events():
		if ev.Kind != EventPaneDied || ev.PaneID != "%41" {
			t.Fatalf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}

	// Now interleave: event, then reply.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		_, _ = tmuxIn.ReadString('\n')
		_, _ = io.WriteString(tmuxOut, "%window-add @7\n%begin 1 1 0\nok\n%end 1 1 0\n")
	}()
	out, err := c.Run(ctx, "noop")
	if err != nil || out != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	select {
	case ev := <-c.Events():
		if ev.Kind != EventWindowAdd || ev.WindowID != "@7" {
			t.Fatalf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no window-add event")
	}
}
