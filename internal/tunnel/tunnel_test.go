package tunnel_test

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/sshclient"
	"github.com/holdemlab/sshdeck/internal/sshtest"
	"github.com/holdemlab/sshdeck/internal/tunnel"
)

func TestParse(t *testing.T) {
	ok := map[string]string{
		"L 8080:localhost:80":         "L 8080:localhost:80",
		"l8080:db.internal:5432":      "L 8080:db.internal:5432",
		"8080:localhost:80":           "L 8080:localhost:80",
		"L 0.0.0.0:8080:10.0.0.5:80":  "L 0.0.0.0:8080:10.0.0.5:80",
		"L *:8080:web:80":             "L 0.0.0.0:8080:web:80",
		"L localhost:9000:x:1":        "L 9000:x:1",
		"L 8080:[2001:db8::1]:443":    "L 8080:[2001:db8::1]:443",
		"D 1080":                      "D 1080",
		"d 0.0.0.0:1080":              "D 0.0.0.0:1080",
		"R 9000:localhost:3000":       "R 9000:localhost:3000",
		"R 0.0.0.0:9000:127.0.0.1:22": "R 0.0.0.0:9000:127.0.0.1:22",
	}
	for in, want := range ok {
		sp, err := tunnel.Parse(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got := sp.String(); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
		// String output parses back to the same spec
		again, err := tunnel.Parse(sp.String())
		if err != nil || again != sp {
			t.Errorf("round trip of %q: %+v, %v", sp.String(), again, err)
		}
	}
	for _, bad := range []string{"", "L", "L 8080", "L 8080:host", "L x:host:80", "L 8080:host:99999", "D 1080:host:80", "L 8080::80"} {
		if _, err := tunnel.Parse(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

// echoService is the "closed" service only reachable from the server side.
func echoService(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					io.WriteString(c, "echo: "+line)
				}
			}()
		}
	}()
	return ln.Addr().String()
}

type yes struct{}

func (yes) Ask(prompt string, echo bool) (string, error) {
	if strings.Contains(prompt, "password") {
		return "pw", nil
	}
	return "yes", nil
}
func (yes) Info(string) {}

func connect(t *testing.T) *sshclient.Conn {
	t.Helper()
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "pw"}})
	host, port, _ := net.SplitHostPort(srv.Addr)
	p, _ := strconv.Atoi(port)
	s := config.DefaultSettings()
	s.KnownHosts = filepath.Join(t.TempDir(), "known_hosts")
	s.UseAgent, s.Identities, s.Keepalive = false, nil, 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := sshclient.Dial(ctx, []sshclient.Hop{{Name: "srv", Host: host, Port: p, User: "u"}}, s, yes{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func roundTrip(t *testing.T, c net.Conn, msg string) {
	t.Helper()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, msg+"\n")
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if line != "echo: "+msg+"\n" {
		t.Fatalf("got %q", line)
	}
}

func spec(t *testing.T, s string) tunnel.Spec {
	sp, err := tunnel.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return sp
}

func TestLocalForward(t *testing.T) {
	conn := connect(t)
	svc := echoService(t)
	f := tunnel.New(spec(t, "L 0:"+svc))
	var events []string
	f.OnEvent = func(e tunnel.Event) { events = append(events, e.Text) }
	if err := f.Start(conn.Client); err != nil {
		t.Fatal(err)
	}
	st := f.Stats()
	if st.State != tunnel.Listening || !strings.HasPrefix(st.Addr, "127.0.0.1:") {
		t.Fatalf("stats = %+v", st)
	}
	c, err := net.Dial("tcp", st.Addr)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c, "hello")
	if f.Stats().Active != 1 {
		t.Errorf("active = %d", f.Stats().Active)
	}

	f.Stop()
	if f.Stats().State != tunnel.Off {
		t.Errorf("state after stop = %v", f.Stats().State)
	}
	// open connections are closed and the port is gone
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Error("connection survived Stop")
	}
	if _, err := net.DialTimeout("tcp", st.Addr, time.Second); err == nil {
		t.Error("port still open after Stop")
	}
}

func TestLocalForwardPortInUse(t *testing.T) {
	conn := connect(t)
	busy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	f := tunnel.New(spec(t, "L "+strconv.Itoa(port)+":localhost:80"))
	if err := f.Start(conn.Client); err == nil {
		t.Fatal("expected address in use")
	}
	if st := f.Stats(); st.State != tunnel.Failed || !strings.Contains(st.Err.Error(), "in use") {
		t.Errorf("stats = %+v", st)
	}
}

func TestUnreachableDestination(t *testing.T) {
	conn := connect(t)
	dead, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := dead.Addr().String()
	dead.Close()
	f := tunnel.New(spec(t, "L 0:"+addr))
	got := make(chan string, 4)
	f.OnEvent = func(e tunnel.Event) {
		if e.Err {
			got <- e.Text
		}
	}
	f.Start(conn.Client)
	defer f.Stop()
	c, _ := net.Dial("tcp", f.Stats().Addr)
	defer c.Close()
	select {
	case msg := <-got:
		if !strings.Contains(msg, "cannot reach") {
			t.Errorf("event = %q", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no error event")
	}
}

func TestDynamicSOCKS5(t *testing.T) {
	conn := connect(t)
	svc := echoService(t)
	f := tunnel.New(spec(t, "D 0"))
	if err := f.Start(conn.Client); err != nil {
		t.Fatal(err)
	}
	defer f.Stop()
	c, err := net.Dial("tcp", f.Stats().Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))

	// greeting: v5, 1 method, no-auth
	c.Write([]byte{5, 1, 0})
	resp := make([]byte, 2)
	io.ReadFull(c, resp)
	if resp[0] != 5 || resp[1] != 0 {
		t.Fatalf("greeting reply %v", resp)
	}
	// CONNECT by domain name "localhost"
	host, portStr, _ := net.SplitHostPort(svc)
	_ = host
	port, _ := strconv.Atoi(portStr)
	req := []byte{5, 1, 0, 3, byte(len("localhost"))}
	req = append(req, "localhost"...)
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	c.Write(req)
	reply := make([]byte, 10)
	io.ReadFull(c, reply)
	if reply[1] != 0 {
		t.Fatalf("connect reply %v", reply)
	}
	roundTrip(t, c, "via socks")
	if st := f.Stats(); st.Total != 1 || st.Active != 1 {
		t.Errorf("SOCKS stats = %+v", st)
	}
}

func TestDynamicSOCKS4a(t *testing.T) {
	conn := connect(t)
	svc := echoService(t)
	f := tunnel.New(spec(t, "D 0"))
	f.Start(conn.Client)
	defer f.Stop()
	c, _ := net.Dial("tcp", f.Stats().Addr)
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	_, portStr, _ := net.SplitHostPort(svc)
	port, _ := strconv.Atoi(portStr)
	req := []byte{4, 1}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	req = append(req, 0, 0, 0, 1, 0) // 0.0.0.1 → 4a, empty user id
	req = append(req, "127.0.0.1"...)
	req = append(req, 0)
	c.Write(req)
	reply := make([]byte, 8)
	io.ReadFull(c, reply)
	if reply[1] != 0x5a {
		t.Fatalf("SOCKS4 reply %v", reply)
	}
	roundTrip(t, c, "via socks4a")
}

func TestRemoteForward(t *testing.T) {
	conn := connect(t)
	svc := echoService(t) // "local" service the server should reach
	f := tunnel.New(spec(t, "R 0:"+svc))
	if err := f.Start(conn.Client); err != nil {
		t.Fatal(err)
	}
	defer f.Stop()
	addr := f.Stats().Addr // port opened on the server side
	_, port, _ := net.SplitHostPort(addr)
	if port == "0" || port == "" {
		t.Fatalf("remote addr = %q", addr)
	}
	c, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	roundTrip(t, c, "from the server side")
}

func TestConnectionLossMarksRemoteFailed(t *testing.T) {
	conn := connect(t)
	f := tunnel.New(spec(t, "R 0:127.0.0.1:1"))
	if err := f.Start(conn.Client); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for f.Stats().State == tunnel.Listening && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.Stats().State != tunnel.Failed {
		t.Errorf("state = %v", f.Stats().State)
	}
}
