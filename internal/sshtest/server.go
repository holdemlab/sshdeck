// Package sshtest is a small in-process SSH server for tests: password and
// public key auth, pty sessions, window-change and direct-tcpip (so it can
// act as a jump host).
package sshtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Pty describes the pty a client requested.
type Pty struct {
	Term          string
	Width, Height int
}

// ShellFunc runs a session. Window changes arrive on resize.
type ShellFunc func(ch ssh.Channel, pty Pty, command string, resize <-chan Pty) uint32

// Server is a running test server.
type Server struct {
	Addr    string
	HostKey ssh.PublicKey

	Passwords      map[string]string // user -> password
	AuthorizedKeys []ssh.PublicKey   // accepted for any user
	// KeyboardInteractive asks for the password via keyboard-interactive
	// ("Password: ") instead of the password method; OTP adds a second
	// question "Verification code: " that must be answered with OTP.
	KeyboardInteractive bool
	OTP                 string
	Shell               ShellFunc

	ln     net.Listener
	mu     sync.Mutex
	Logins []string // "user/method" for each successful login
	Sizes  []Pty    // pty sizes seen (request + window changes)
}

// Start listens on 127.0.0.1 with a fresh host key.
func Start(t testing.TB, s *Server) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Serve(ln); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return s
}

// Serve starts accepting connections on ln in the background.
func (s *Server) Serve(ln net.Listener) error {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return err
	}
	s.HostKey = signer.PublicKey()
	s.ln = ln
	s.Addr = ln.Addr().String()

	cfg := &ssh.ServerConfig{}
	if s.KeyboardInteractive {
		cfg.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, client ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			ans, err := client(c.User(), "", []string{"Password: "}, []bool{false})
			if err != nil || len(ans) != 1 {
				return nil, fmt.Errorf("no answer")
			}
			if want, ok := s.Passwords[c.User()]; !ok || want != ans[0] {
				return nil, fmt.Errorf("bad password")
			}
			if s.OTP != "" {
				ans, err = client(c.User(), "", []string{"Verification code: "}, []bool{false})
				if err != nil || len(ans) != 1 || ans[0] != s.OTP {
					return nil, fmt.Errorf("bad code")
				}
			}
			s.login(c.User(), "keyboard-interactive")
			return nil, nil
		}
	} else {
		cfg.PasswordCallback = func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if want, ok := s.Passwords[c.User()]; ok && want == string(pw) {
				s.login(c.User(), "password")
				return nil, nil
			}
			return nil, fmt.Errorf("bad password")
		}
	}
	cfg.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		for _, k := range s.AuthorizedKeys {
			if string(k.Marshal()) == string(key.Marshal()) {
				return &ssh.Permissions{Extensions: map[string]string{"pubkey": "1"}}, nil
			}
		}
		return nil, fmt.Errorf("unknown key")
	}
	cfg.AddHostKey(signer)
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handle(nc, cfg)
		}
	}()
	return nil
}

func (s *Server) login(user, method string) {
	s.mu.Lock()
	s.Logins = append(s.Logins, user+"/"+method)
	s.mu.Unlock()
}

// LoginList returns a copy of Logins.
func (s *Server) LoginList() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.Logins...)
}

// SizeList returns a copy of Sizes.
func (s *Server) SizeList() []Pty {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Pty(nil), s.Sizes...)
}

func (s *Server) handle(nc net.Conn, cfg *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		nc.Close()
		return
	}
	if conn.Permissions != nil && conn.Permissions.Extensions["pubkey"] == "1" {
		s.login(conn.User(), "publickey")
	}
	go s.globalRequests(conn, reqs)
	for nch := range chans {
		switch nch.ChannelType() {
		case "session":
			go s.session(nch)
		case "direct-tcpip":
			go directTCPIP(nch)
		default:
			nch.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func (s *Server) session(nch ssh.NewChannel) {
	ch, reqs, err := nch.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	var pty Pty
	resize := make(chan Pty, 16)
	started := false
	for req := range reqs {
		switch req.Type {
		case "pty-req":
			var p struct {
				Term         string
				W, H, PW, PH uint32
				Modes        string
			}
			ssh.Unmarshal(req.Payload, &p)
			pty = Pty{Term: p.Term, Width: int(p.W), Height: int(p.H)}
			s.mu.Lock()
			s.Sizes = append(s.Sizes, pty)
			s.mu.Unlock()
			req.Reply(true, nil)
		case "window-change":
			var p struct{ W, H, PW, PH uint32 }
			ssh.Unmarshal(req.Payload, &p)
			sz := Pty{Term: pty.Term, Width: int(p.W), Height: int(p.H)}
			s.mu.Lock()
			s.Sizes = append(s.Sizes, sz)
			s.mu.Unlock()
			select {
			case resize <- sz:
			default:
			}
		case "shell", "exec":
			if started {
				req.Reply(false, nil)
				continue
			}
			started = true
			var command string
			if req.Type == "exec" {
				var p struct{ Command string }
				ssh.Unmarshal(req.Payload, &p)
				command = p.Command
			}
			req.Reply(true, nil)
			shell := s.Shell
			if shell == nil {
				shell = EchoShell
			}
			go func() {
				code := shell(ch, pty, command, resize)
				status := make([]byte, 4)
				binary.BigEndian.PutUint32(status, code)
				ch.SendRequest("exit-status", false, status)
				ch.Close()
			}()
		default:
			req.Reply(false, nil)
		}
	}
}

// EchoShell greets, then echoes input lines back until "exit".
func EchoShell(ch ssh.Channel, pty Pty, command string, _ <-chan Pty) uint32 {
	if command != "" {
		fmt.Fprintf(ch, "ran: %s\r\n", command)
		return 0
	}
	fmt.Fprintf(ch, "welcome %s %dx%d\r\n$ ", pty.Term, pty.Width, pty.Height)
	var line []byte
	buf := make([]byte, 256)
	for {
		n, err := ch.Read(buf)
		if err != nil {
			return 1
		}
		for _, b := range buf[:n] {
			switch b {
			case '\r', '\n':
				if string(line) == "exit" {
					io.WriteString(ch, "\r\nbye\r\n")
					return 0
				}
				fmt.Fprintf(ch, "\r\nyou said: %s\r\n$ ", line)
				line = line[:0]
			default:
				line = append(line, b)
				ch.Write([]byte{b})
			}
		}
	}
}

func directTCPIP(nch ssh.NewChannel) {
	var p struct {
		Host     string
		Port     uint32
		OrigHost string
		OrigPort uint32
	}
	if err := ssh.Unmarshal(nch.ExtraData(), &p); err != nil {
		nch.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	target, err := net.Dial("tcp", net.JoinHostPort(p.Host, fmt.Sprint(p.Port)))
	if err != nil {
		nch.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, err := nch.Accept()
	if err != nil {
		target.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	go func() { io.Copy(ch, target); ch.CloseWrite() }()
	io.Copy(target, ch)
	target.Close()
	ch.Close()
}

// globalRequests handles remote port forwarding (ssh -R).
func (s *Server) globalRequests(conn *ssh.ServerConn, reqs <-chan *ssh.Request) {
	var mu sync.Mutex
	listeners := map[string]net.Listener{}
	defer func() {
		mu.Lock()
		for _, ln := range listeners {
			ln.Close()
		}
		mu.Unlock()
	}()
	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			var p struct {
				Addr string
				Port uint32
			}
			if err := ssh.Unmarshal(req.Payload, &p); err != nil {
				req.Reply(false, nil)
				continue
			}
			ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(p.Port)))
			if err != nil {
				req.Reply(false, nil)
				continue
			}
			port := uint32(ln.Addr().(*net.TCPAddr).Port)
			key := fmt.Sprintf("%s:%d", p.Addr, p.Port)
			if p.Port == 0 {
				key = fmt.Sprintf("%s:%d", p.Addr, port)
			}
			mu.Lock()
			listeners[key] = ln
			mu.Unlock()
			reply := make([]byte, 4)
			binary.BigEndian.PutUint32(reply, port)
			req.Reply(true, reply)
			go func(bindAddr string) {
				for {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					go func() {
						defer c.Close()
						ra := c.RemoteAddr().(*net.TCPAddr)
						payload := ssh.Marshal(struct {
							Addr       string
							Port       uint32
							OriginAddr string
							OriginPort uint32
						}{bindAddr, port, ra.IP.String(), uint32(ra.Port)})
						ch, chReqs, err := conn.OpenChannel("forwarded-tcpip", payload)
						if err != nil {
							return
						}
						go ssh.DiscardRequests(chReqs)
						go func() { io.Copy(ch, c); ch.CloseWrite() }()
						io.Copy(c, ch)
						ch.Close()
					}()
				}
			}(p.Addr)
		case "cancel-tcpip-forward":
			var p struct {
				Addr string
				Port uint32
			}
			ssh.Unmarshal(req.Payload, &p)
			key := fmt.Sprintf("%s:%d", p.Addr, p.Port)
			mu.Lock()
			if ln, ok := listeners[key]; ok {
				ln.Close()
				delete(listeners, key)
			}
			mu.Unlock()
			req.Reply(true, nil)
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}
