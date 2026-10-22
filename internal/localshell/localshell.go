// Package localshell runs a shell on this computer in a pseudo-terminal, for
// sshdeck's local tabs.
package localshell

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Supported reports whether local tabs work on this system (they need a
// Unix pseudo-terminal).
func Supported() bool {
	return runtime.GOOS != "windows" && runtime.GOOS != "plan9" && runtime.GOOS != "js"
}

// ErrUnsupported is returned by Start where Supported is false.
var ErrUnsupported = errors.New("local shell is not supported on this system")

// Shell is a running local shell.
type Shell struct {
	cmd    *exec.Cmd
	pty    *os.File
	copied chan struct{} // closed when all output has been read
	once   sync.Once
}

// Argv is what Start runs: $SHELL (or /bin/sh) for an empty command,
// otherwise the command through /bin/sh -c.
func Argv(command string) []string {
	if command = strings.TrimSpace(command); command != "" {
		return []string{"/bin/sh", "-c", command}
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	return []string{sh}
}

// Describe is a short label for the status bar: "zsh", "fish -l".
func Describe(command string) string {
	if command = strings.TrimSpace(command); command != "" {
		return command
	}
	return filepath.Base(Argv("")[0])
}

// Start runs command (see Argv) with a terminal of the given size; its
// output goes to out. term is the TERM value programs see.
func Start(command, term string, width, height int, out io.Writer) (*Shell, error) {
	if !Supported() {
		return nil, ErrUnsupported
	}
	argv := Argv(command)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = environ(term)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(max(height, 1)), Cols: uint16(max(width, 1))})
	if err != nil {
		return nil, err
	}
	s := &Shell{cmd: cmd, pty: f, copied: make(chan struct{})}
	go func() {
		_, _ = io.Copy(out, f) // ends with EIO once the shell is gone
		close(s.copied)
	}()
	return s, nil
}

// environ is our environment for the shell, with the terminal described.
func environ(term string) []string {
	if term == "" {
		term = "xterm-256color"
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "TERM", "COLORTERM", "COLUMNS", "LINES", "SSHDECK":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "TERM="+term, "COLORTERM=truecolor", "SSHDECK=1")
}

// Write sends input (keys, pastes) to the shell.
func (s *Shell) Write(p []byte) (int, error) { return s.pty.Write(p) }

// Resize changes the terminal size.
func (s *Shell) Resize(width, height int) error {
	return pty.Setsize(s.pty, &pty.Winsize{Rows: uint16(max(height, 1)), Cols: uint16(max(width, 1))})
}

// Wait waits for the shell to exit and its output to be read, and returns
// its exit error (*exec.ExitError for a non-zero status).
func (s *Shell) Wait() error {
	err := s.cmd.Wait()
	select { // background jobs may keep the terminal open: don't hang on them
	case <-s.copied:
	case <-time.After(500 * time.Millisecond):
	}
	s.closePty()
	return err
}

// Close hangs up, like closing a terminal window: the shell gets SIGHUP and
// is killed if it is still there two seconds later.
func (s *Shell) Close() error {
	if p := s.cmd.Process; p != nil {
		_ = p.Signal(syscall.SIGHUP)
		go func() {
			time.Sleep(2 * time.Second)
			if s.cmd.ProcessState == nil {
				_ = p.Kill()
			}
		}()
	}
	s.closePty()
	return nil
}

func (s *Shell) closePty() { s.once.Do(func() { _ = s.pty.Close() }) }
