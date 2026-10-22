package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// Client is what a forward needs from the SSH connection; *ssh.Client
// implements it.
type Client interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	Listen(network, addr string) (net.Listener, error)
}

// State of a forward.
type State int

const (
	Off State = iota
	Listening
	Failed
)

// Event is reported for things worth logging.
type Event struct {
	Forward *Forward
	Text    string
	Err     bool
}

// Forward is one running (or stopped) tunnel. Methods are goroutine-safe.
type Forward struct {
	Spec Spec

	// Wanted means "should be on whenever the connection is up".
	Wanted atomic.Bool
	// FromConfig marks forwards defined in sessions.yaml.
	FromConfig bool

	OnEvent func(Event)

	mu     sync.Mutex
	state  State
	err    error
	ln     net.Listener
	conns  map[net.Conn]struct{}
	cancel context.CancelFunc

	active atomic.Int64
	total  atomic.Int64
	bytes  atomic.Int64
}

// New returns a stopped forward.
func New(spec Spec) *Forward { return &Forward{Spec: spec} }

// Stats is a snapshot for display.
type Stats struct {
	State  State
	Err    error
	Active int64
	Total  int64
	Bytes  int64
	Addr   string // actual listen address (useful for port 0)
}

// Stats returns the current state.
func (f *Forward) Stats() Stats {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := Stats{State: f.state, Err: f.err, Active: f.active.Load(), Total: f.total.Load(), Bytes: f.bytes.Load()}
	if f.ln != nil {
		st.Addr = f.ln.Addr().String()
	}
	return st
}

func (f *Forward) event(failed bool, format string, a ...any) {
	if f.OnEvent != nil {
		f.OnEvent(Event{Forward: f, Text: fmt.Sprintf(format, a...), Err: failed})
	}
}

// Start opens the listener and begins accepting. It is a no-op if the
// forward is already listening.
func (f *Forward) Start(c Client) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == Listening {
		return nil
	}
	var ln net.Listener
	var err error
	if f.Spec.Kind == Remote {
		ln, err = c.Listen("tcp", f.Spec.BindAddr())
	} else {
		ln, err = net.Listen("tcp", f.Spec.BindAddr())
	}
	if err != nil {
		f.state, f.err = Failed, cleanErr(err)
		go f.event(true, "%s: %v", f.Spec, f.err)
		return f.err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.state, f.err, f.ln, f.cancel = Listening, nil, ln, cancel
	f.conns = map[net.Conn]struct{}{}
	go f.event(false, "%s listening on %s", f.Spec, ln.Addr())
	go f.accept(ctx, c, ln)
	return nil
}

// Stop closes the listener and every connection through it.
func (f *Forward) Stop() {
	f.mu.Lock()
	ln, cancel, conns := f.ln, f.cancel, f.conns
	f.ln, f.cancel, f.conns = nil, nil, nil
	if f.state == Listening {
		f.state = Off
	}
	f.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if ln != nil {
		ln.Close()
	}
	for c := range conns {
		c.Close()
	}
}

// Reset clears a failure so the forward shows as off.
func (f *Forward) Reset() {
	f.mu.Lock()
	if f.state == Failed {
		f.state, f.err = Off, nil
	}
	f.mu.Unlock()
}

func (f *Forward) accept(ctx context.Context, c Client, ln net.Listener) {
	for {
		in, err := ln.Accept()
		if err != nil {
			f.mu.Lock()
			if f.ln == ln { // closed underneath us (connection lost)
				f.state, f.err, f.ln = Failed, errors.New("listener closed"), nil
			}
			f.mu.Unlock()
			return
		}
		if !f.track(in) {
			in.Close()
			return
		}
		go f.serve(ctx, c, in)
	}
}

func (f *Forward) track(c net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conns == nil {
		return false
	}
	f.conns[c] = struct{}{}
	return true
}

func (f *Forward) untrack(c net.Conn) {
	f.mu.Lock()
	if f.conns != nil {
		delete(f.conns, c)
	}
	f.mu.Unlock()
}

func (f *Forward) serve(ctx context.Context, c Client, in net.Conn) {
	defer f.untrack(in)
	defer in.Close()

	var out net.Conn
	var dest string
	var err error
	switch f.Spec.Kind {
	case Local:
		dest = f.Spec.DestAddr()
		out, err = c.DialContext(ctx, "tcp", dest)
	case Remote:
		dest = f.Spec.DestAddr()
		var d net.Dialer
		out, err = d.DialContext(ctx, "tcp", dest)
	case Dynamic:
		var v4 bool
		dest, v4, err = socksHandshake(in)
		if err != nil {
			f.event(true, "%s: SOCKS: %v", f.Spec, err)
			return
		}
		out, err = c.DialContext(ctx, "tcp", dest)
		socksReply(in, v4, err)
	}
	if err != nil {
		f.event(true, "%s: cannot reach %s: %v", f.Spec, dest, cleanErr(err))
		return
	}
	defer out.Close()
	if !f.track(out) {
		return
	}
	defer f.untrack(out)

	f.active.Add(1)
	f.total.Add(1)
	defer f.active.Add(-1)

	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		n, _ := io.Copy(dst, src)
		f.bytes.Add(n)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(out, in)
	go cp(in, out)
	<-done
	<-done
}

func cleanErr(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Err != nil {
		return op.Err
	}
	return err
}
