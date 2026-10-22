package localshell

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

type buffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestRunsCommandInATerminal(t *testing.T) {
	if !Supported() {
		t.Skip("no pty here")
	}
	var out buffer
	sh, err := Start(`printf "%s %s %s\n" "$TERM" "$SSHDECK" "$(stty size)"`, "xterm-test", 77, 21, &out)
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "xterm-test 1 21 77") {
		t.Errorf("output %q", got)
	}
}

func TestInputResizeAndExitStatus(t *testing.T) {
	if !Supported() {
		t.Skip("no pty here")
	}
	var out buffer
	sh, err := Start(`read line; stty size; exit 3`, "", 80, 24, &out)
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	sh.Write([]byte("hello\r"))
	err = sh.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("exit: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "hello") || !strings.Contains(got, "30 100") {
		t.Errorf("output %q", got)
	}
}

func TestCloseHangsUp(t *testing.T) {
	if !Supported() {
		t.Skip("no pty here")
	}
	var out buffer
	sh, err := Start("sleep 30", "", 80, 24, &out)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- sh.Wait() }()
	sh.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shell still running after Close")
	}
}

func TestArgv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := Argv(""); len(got) != 1 || got[0] != "/usr/bin/fish" {
		t.Errorf("Argv(\"\") = %q", got)
	}
	if got := Argv("zsh -l"); strings.Join(got, "|") != "/bin/sh|-c|zsh -l" {
		t.Errorf("Argv(zsh -l) = %q", got)
	}
	if Describe("") != "fish" || Describe(" htop ") != "htop" {
		t.Errorf("Describe: %q %q", Describe(""), Describe(" htop "))
	}
	t.Setenv("SHELL", "")
	if Argv("")[0] != "/bin/sh" {
		t.Error("no $SHELL should fall back to /bin/sh")
	}
}
