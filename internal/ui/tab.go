package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"golang.org/x/crypto/ssh"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/localshell"
	"github.com/holdemlab/sshdeck/internal/sshclient"
	"github.com/holdemlab/sshdeck/internal/tunnel"
)

type tabState int

const (
	stateConnecting tabState = iota
	stateConnected
	stateClosed
)

// tabEventMsg asks the UI to redraw because a tab changed.
type tabEventMsg struct{ id int }

// Notifier delivers messages from background goroutines to the program.
// main wires Send to tea.Program.Send.
type Notifier struct {
	mu   sync.Mutex
	send func(tea.Msg)
}

// SetSend sets the delivery function.
func (n *Notifier) SetSend(f func(tea.Msg)) {
	n.mu.Lock()
	n.send = f
	n.mu.Unlock()
}

// Send delivers msg if a program is attached. It never blocks: Program.Send
// would deadlock when called from inside Update.
func (n *Notifier) Send(msg tea.Msg) {
	if n == nil {
		return
	}
	n.mu.Lock()
	f := n.send
	n.mu.Unlock()
	if f != nil {
		go f(msg)
	}
}

// shellConn is the far end of a tab: an SSH shell or a local one.
type shellConn interface {
	Write(p []byte) (int, error)
	Resize(width, height int) error
	Close() error
}

// Tab is one SSH connection (or a local shell) rendered through a terminal
// emulator.
type Tab struct {
	id      int
	session config.Resolved
	emu     *vt.SafeEmulator
	notify  *Notifier
	dirty   atomic.Bool

	mu     sync.Mutex
	state  tabState
	reason string // why the tab closed
	conn   *sshclient.Conn
	shell  shellConn
	ctx    context.Context // current connection attempt
	cancel context.CancelFunc
	prompt *linePrompt
	route  string // "jumper@proxy → deploy@web-01"

	// tunnels
	tunnelOnly bool // no shell: the connection only carries forwards
	forwards   []*tunnel.Forward
	client     *ssh.Client // set while connected, for starting forwards
	lastEvent  string      // last tunnel error, shown in the status bar

	scroll int // lines scrolled back; 0 = live view (UI goroutine only)

	local bool // a shell on this computer, no SSH

	// mouseModes has a bit for every mouse reporting mode the program in
	// the tab turned on (vim with mouse=a, htop, mc…); 0: it wants no mouse.
	mouseModes atomic.Uint32
}

// linePrompt is an in-tab question (password, host key, passphrase).
type linePrompt struct {
	echo  bool
	buf   []rune
	reply chan promptReply
}

type promptReply struct {
	text string
	err  error
}

func newTab(id int, s config.Resolved, width, height, scrollback int, n *Notifier, tunnelOnly bool) *Tab {
	emu := vt.NewSafeEmulator(max(width, 1), max(height, 1))
	themeEmulator(emu, curTheme)
	emu.SetScrollbackSize(scrollback)
	t := &Tab{id: id, session: s, emu: emu, notify: n, tunnelOnly: tunnelOnly}
	// Track mouse reporting. The callbacks run inside Write, under the
	// emulator's lock: they only touch an atomic.
	emu.SetCallbacks(vt.Callbacks{
		EnableMode:  func(m ansi.Mode) { t.trackMouse(m, true) },
		DisableMode: func(m ansi.Mode) { t.trackMouse(m, false) },
	})
	for _, fw := range s.Forwards {
		if sp, err := tunnel.Parse(fw); err == nil {
			f := t.newForward(sp)
			f.FromConfig = true
		}
	}
	go t.pumpInput()
	return t
}

var trackedMouseModes = []ansi.Mode{ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseHighlight, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent}

// trackMouse notes a program turning mouse reporting on or off.
func (t *Tab) trackMouse(m ansi.Mode, on bool) {
	for i, mm := range trackedMouseModes {
		if m == mm {
			bit := uint32(1) << i
			if on {
				t.mouseModes.Or(bit)
			} else {
				t.mouseModes.And(^bit)
			}
			return
		}
	}
}

// wantsMouse reports whether the program in the tab handles the mouse.
func (t *Tab) wantsMouse() bool { return t.mouseModes.Load() > 0 }

// title is the label in the tab bar.
func (t *Tab) title() string {
	if t.tunnelOnly {
		return "⇄ " + t.session.Name
	}
	return t.session.Name
}

func (t *Tab) getState() (tabState, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state, t.reason
}

// changed schedules a redraw; bursts of output collapse into one message.
func (t *Tab) changed() {
	if t.dirty.CompareAndSwap(false, true) {
		t.notify.Send(tabEventMsg{id: t.id})
	}
}

// Write receives remote output.
func (t *Tab) Write(p []byte) (int, error) {
	n, err := t.emu.Write(p)
	t.changed()
	return n, err
}

// pumpInput forwards what the emulator produces (encoded keys, replies to
// terminal queries) to the remote shell.
func (t *Tab) pumpInput() {
	buf := make([]byte, 4096)
	for {
		n, err := t.emu.Read(buf)
		if n > 0 {
			t.mu.Lock()
			sh := t.shell
			t.mu.Unlock()
			if sh != nil {
				_, _ = sh.Write(buf[:n])
			}
		}
		if err != nil {
			return
		}
	}
}

// print writes local text (status lines) into the terminal.
func (t *Tab) print(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	_, _ = t.emu.Write([]byte(s))
	t.changed()
}

// ---- sshclient.Prompter ----

// Info implements sshclient.Prompter.
func (t *Tab) Info(msg string) { t.print("\x1b[2m" + msg + "\x1b[0m\n") }

// Ask implements sshclient.Prompter: the prompt is printed in the tab and
// the answer typed there.
func (t *Tab) Ask(prompt string, echo bool) (string, error) {
	p := &linePrompt{echo: echo, reply: make(chan promptReply, 1)}
	t.mu.Lock()
	t.prompt = p
	ctx := t.ctx
	t.mu.Unlock()
	t.print(prompt)
	select {
	case r := <-p.reply:
		return r.text, r.err
	case <-ctx.Done():
		t.mu.Lock()
		if t.prompt == p {
			t.prompt = nil
		}
		t.mu.Unlock()
		return "", sshclient.ErrCanceled
	}
}

// ---- lifecycle ----

// connect dials the session in the background. sec remembers passwords
// (nil: ask every time).
func (t *Tab) connect(cfg *config.Config, s config.Settings, sec *sshclient.Secrets) {
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	if t.state == stateConnected || (t.state == stateConnecting && t.cancel != nil) {
		t.mu.Unlock()
		cancel()
		return
	}
	t.state, t.reason, t.ctx, t.cancel = stateConnecting, "", ctx, cancel
	t.mu.Unlock()
	t.mouseModes.Store(0) // a new program starts without mouse reporting
	t.changed()

	sess := t.session // snapshot: the UI may update t.session while connected
	go func() {
		err := t.run(ctx, cfg, s, sess, sec)
		cancel()
		failed := err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, sshclient.ErrCanceled)
		t.mu.Lock()
		t.state, t.shell, t.conn, t.cancel = stateClosed, nil, nil, nil
		t.prompt = nil
		t.reason = T("closed")
		if failed {
			t.reason = err.Error()
		}
		t.mu.Unlock()
		if failed {
			t.print("\r\n\x1b[31m✗ " + err.Error() + "\x1b[0m\n")
		} else if t.local {
			t.print("\r\n\x1b[2m" + T("The shell has exited.") + "\x1b[0m\n")
		} else {
			t.print("\r\n\x1b[2m" + T("Connection to %s closed.", sess.Name) + "\x1b[0m\n")
		}
		t.print("\x1b[2m" + T("Press r to reconnect.") + "\x1b[0m\n")
	}()
}

func (t *Tab) run(ctx context.Context, cfg *config.Config, s config.Settings, sess config.Resolved, sec *sshclient.Secrets) error {
	if t.local {
		return t.runLocal(ctx, s)
	}
	hops, err := sshclient.Plan(cfg, sess)
	if err != nil {
		return err
	}
	var route []string
	for _, h := range hops {
		route = append(route, h.User+"@"+h.Host)
	}
	t.mu.Lock()
	t.route = strings.Join(route, " → ")
	t.mu.Unlock()

	conn, err := sshclient.DialWith(ctx, hops, s, t, sec)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer t.stopForwards()
	t.mu.Lock()
	t.client, t.conn = conn.Client, conn
	t.mu.Unlock()
	t.startForwards()

	if t.tunnelOnly {
		return t.runTunnelsOnly(ctx, conn, s.Prefix)
	}

	w, h := t.emu.Width(), t.emu.Height()
	shell, err := conn.StartShell(s.Term, w, h, sess.Command, t)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.conn, t.shell, t.state = conn, shell, stateConnected
	t.mu.Unlock()
	t.changed()

	done := make(chan error, 1)
	go func() { done <- shell.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		shell.Close()
		return ctx.Err()
	}
	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		if exit.ExitStatus() != 0 {
			return fmt.Errorf("%s", T("remote exited with status %d", exit.ExitStatus()))
		}
		return nil
	}
	var missing *ssh.ExitMissingError
	if errors.As(err, &missing) || errors.Is(err, io.EOF) {
		return errors.New(T("connection lost"))
	}
	return err
}

// runLocal starts a shell on this computer and waits for it to exit.
func (t *Tab) runLocal(ctx context.Context, s config.Settings) error {
	t.mu.Lock()
	t.route = T("local: %s", localshell.Describe(s.LocalCommand))
	t.mu.Unlock()
	sh, err := localshell.Start(s.LocalCommand, s.Term, t.emu.Width(), t.emu.Height(), t)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.shell, t.state = sh, stateConnected
	t.mu.Unlock()
	t.changed()

	done := make(chan error, 1)
	go func() { done <- sh.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		sh.Close()
		<-done
		return ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if code := exit.ExitCode(); code > 0 {
			return fmt.Errorf("%s", T("the shell exited with status %d", code))
		}
		return nil // killed by a signal, e.g. after a hang-up
	}
	return err
}

// close ends the connection (if any) and stops the input pump.
func (t *Tab) close() {
	t.mu.Lock()
	cancel, p := t.cancel, t.prompt
	t.prompt = nil
	t.mu.Unlock()
	if p != nil {
		p.reply <- promptReply{err: sshclient.ErrCanceled}
	}
	if cancel != nil {
		cancel()
	}
	for _, f := range t.forwardList() {
		f.Wanted.Store(false)
		f.Stop()
	}
	// End pumpInput. Closing the input pipe is goroutine-safe, unlike
	// Emulator.Close (SafeEmulator does not lock it).
	if c, ok := t.emu.InputPipe().(io.Closer); ok {
		_ = c.Close()
	}
}

// screenText returns the visible screen as plain text (thread-safe).
func (t *Tab) screenText() string { return ansi.Strip(t.emu.Render()) }

// resize changes the emulator and remote pty size.
func (t *Tab) resize(w, h int) {
	if w < 1 || h < 1 || (w == t.emu.Width() && h == t.emu.Height()) {
		return
	}
	t.emu.Resize(w, h)
	t.mu.Lock()
	sh := t.shell
	t.mu.Unlock()
	if sh != nil {
		_ = sh.Resize(w, h)
	}
}

// ---- input ----

// handleKey routes a key press into the tab. It reports whether the tab
// wants to reconnect.
func (t *Tab) handleKey(k tea.KeyPressMsg) (reconnect bool) {
	t.mu.Lock()
	p, state := t.prompt, t.state
	t.mu.Unlock()

	if p != nil {
		t.promptKey(p, k)
		return false
	}
	switch state {
	case stateConnected:
		t.scroll = 0
		t.emu.SendKey(uv.KeyPressEvent(k))
	case stateClosed:
		if s := k.String(); s == "r" || s == "enter" {
			return true
		}
	}
	return false
}

func (t *Tab) promptKey(p *linePrompt, k tea.KeyPressMsg) {
	switch k.String() {
	case "enter":
		t.print("\n")
		t.finishPrompt(p, promptReply{text: string(p.buf)})
	case "ctrl+c", "esc", "ctrl+d":
		t.print("^C\n")
		t.finishPrompt(p, promptReply{err: sshclient.ErrCanceled})
	case "backspace", "ctrl+h":
		if len(p.buf) > 0 {
			p.buf = p.buf[:len(p.buf)-1]
			if p.echo {
				t.print("\b \b")
			}
		}
	case "ctrl+u":
		if p.echo {
			t.print(strings.Repeat("\b \b", len(p.buf)))
		}
		p.buf = p.buf[:0]
	default:
		if k.Text != "" {
			t.promptText(p, k.Text)
		}
	}
}

func (t *Tab) promptText(p *linePrompt, s string) {
	s = strings.NewReplacer("\r", "", "\n", "").Replace(s)
	p.buf = append(p.buf, []rune(s)...)
	if p.echo {
		t.print(s)
	}
}

func (t *Tab) finishPrompt(p *linePrompt, r promptReply) {
	t.mu.Lock()
	if t.prompt == p {
		t.prompt = nil
	}
	t.mu.Unlock()
	p.reply <- r
}

// paste sends pasted text (bracketed if the remote app asked for it).
func (t *Tab) paste(s string) {
	t.mu.Lock()
	p, state := t.prompt, t.state
	t.mu.Unlock()
	if p != nil {
		t.promptText(p, s)
		return
	}
	if state == stateConnected {
		t.scroll = 0
		t.emu.Paste(s)
	}
}

// sendKey sends a key to the remote side (used for a literal prefix key).
func (t *Tab) sendKey(k tea.KeyPressMsg) {
	t.mu.Lock()
	state := t.state
	t.mu.Unlock()
	if state == stateConnected {
		t.emu.SendKey(uv.KeyPressEvent(k))
	}
}

// connection returns the live connection, or nil.
func (t *Tab) connection() *sshclient.Conn {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != stateConnected {
		return nil
	}
	return t.conn
}

func (t *Tab) routeString() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.route
}

func (t *Tab) live() bool {
	s, _ := t.getState()
	return s != stateClosed
}

// ---- tunnels ----

// runTunnelsOnly keeps the connection open without a shell until it drops
// or the tab is closed.
func (t *Tab) runTunnelsOnly(ctx context.Context, conn *sshclient.Conn, prefix string) error {
	t.mu.Lock()
	t.state = stateConnected
	n := len(t.forwards)
	t.mu.Unlock()
	t.changed()
	t.print("\x1b[32m" + T("Connected.") + "\x1b[0m " + T("Tunnels only, no shell.") + "\n")
	if n == 0 {
		t.print(T("No tunnels defined yet: press %s t, then a to add one.", prefix) + "\n")
	}
	done := make(chan error, 1)
	go func() { done <- conn.Client.Wait() }()
	select {
	case <-done:
		return errors.New(T("connection lost"))
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *Tab) newForward(sp tunnel.Spec) *tunnel.Forward {
	f := tunnel.New(sp)
	f.Wanted.Store(true)
	f.OnEvent = t.tunnelEvent
	t.mu.Lock()
	t.forwards = append(t.forwards, f)
	t.mu.Unlock()
	return f
}

func (t *Tab) tunnelEvent(e tunnel.Event) {
	if t.tunnelOnly {
		color := "\x1b[2m"
		if e.Err {
			color = "\x1b[31m"
		}
		t.print(color + time.Now().Format("15:04:05") + " " + e.Text + "\x1b[0m\n")
		return
	}
	if e.Err {
		t.mu.Lock()
		t.lastEvent = e.Text
		t.mu.Unlock()
	}
	t.changed()
}

// forwardList returns a snapshot of the tab's forwards.
func (t *Tab) forwardList() []*tunnel.Forward {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*tunnel.Forward(nil), t.forwards...)
}

// startForwards starts every wanted forward on the current connection.
func (t *Tab) startForwards() {
	t.mu.Lock()
	c := t.client
	t.mu.Unlock()
	if c == nil {
		return
	}
	for _, f := range t.forwardList() {
		if f.Wanted.Load() {
			_ = f.Start(c)
		}
	}
	t.changed()
}

// stopForwards stops all forwards but remembers which were wanted, so a
// reconnect brings them back.
func (t *Tab) stopForwards() {
	t.mu.Lock()
	t.client = nil
	t.mu.Unlock()
	for _, f := range t.forwardList() {
		f.Stop()
	}
	t.changed()
}

// addForward adds a forward and starts it if connected.
func (t *Tab) addForward(sp tunnel.Spec) error {
	for _, f := range t.forwardList() {
		if f.Spec == sp {
			return fmt.Errorf("%s already exists", sp)
		}
	}
	f := t.newForward(sp)
	t.mu.Lock()
	c := t.client
	t.mu.Unlock()
	if c != nil {
		if err := f.Start(c); err != nil {
			return err
		}
	}
	t.changed()
	return nil
}

// toggleForward switches a forward on or off.
func (t *Tab) toggleForward(f *tunnel.Forward) error {
	if f.Stats().State == tunnel.Listening {
		f.Wanted.Store(false)
		f.Stop()
		t.changed()
		return nil
	}
	f.Wanted.Store(true)
	f.Reset()
	t.mu.Lock()
	c := t.client
	t.mu.Unlock()
	defer t.changed()
	if c == nil {
		return nil // starts on connect
	}
	return f.Start(c)
}

// removeForward stops and drops a forward.
func (t *Tab) removeForward(f *tunnel.Forward) {
	f.Wanted.Store(false)
	f.Stop()
	t.mu.Lock()
	for i, x := range t.forwards {
		if x == f {
			t.forwards = append(t.forwards[:i], t.forwards[i+1:]...)
			break
		}
	}
	t.mu.Unlock()
	t.changed()
}

// listening counts forwards that are up.
func (t *Tab) listening() int {
	n := 0
	for _, f := range t.forwardList() {
		if f.Stats().State == tunnel.Listening {
			n++
		}
	}
	return n
}

func (t *Tab) takeLastEvent() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastEvent
}
