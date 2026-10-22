package sshclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/sshtest"
)

// scripted answers prompts from a queue and records everything.
type scripted struct {
	mu      sync.Mutex
	answers []string
	prompts []string
	infos   []string
}

func (s *scripted) Ask(prompt string, echo bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
	if len(s.answers) == 0 {
		return "", ErrCanceled
	}
	a := s.answers[0]
	s.answers = s.answers[1:]
	return a, nil
}

func (s *scripted) Info(msg string) {
	s.mu.Lock()
	s.infos = append(s.infos, msg)
	s.mu.Unlock()
}

func testSettings(t *testing.T) config.Settings {
	s := config.DefaultSettings()
	s.KnownHosts = filepath.Join(t.TempDir(), "known_hosts")
	s.UseAgent = false
	s.Identities = nil
	s.Keepalive = 0
	s.ConnectTimeout = config.Duration(5 * time.Second)
	return s
}

func hopFor(srv *sshtest.Server, user string) Hop {
	host, port, _ := net.SplitHostPort(srv.Addr)
	p, _ := strconv.Atoi(port)
	return Hop{Name: user + "-srv", Host: host, Port: p, User: user}
}

func dial(t *testing.T, s config.Settings, ask *scripted, hops ...Hop) (*Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // the passphrase KDF is slow under -race
	defer cancel()
	return Dial(ctx, hops, s, ask)
}

func TestPasswordAndTrustOnFirstUse(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"alice": "secret"}})
	s := testSettings(t)

	ask := &scripted{answers: []string{"yes", "secret"}}
	c, err := dial(t, s, ask, hopFor(srv, "alice"))
	if err != nil {
		t.Fatalf("dial: %v (prompts %q)", err, ask.prompts)
	}
	c.Close()
	if !strings.Contains(ask.prompts[0], "authenticity of host") || !strings.Contains(ask.prompts[0], ssh.FingerprintSHA256(srv.HostKey)) {
		t.Errorf("host key prompt = %q", ask.prompts[0])
	}
	if !strings.Contains(ask.prompts[1], "alice@127.0.0.1's password") {
		t.Errorf("password prompt = %q", ask.prompts[1])
	}
	data, _ := os.ReadFile(s.KnownHosts)
	if !strings.Contains(string(data), "ssh-ed25519") {
		t.Fatalf("known_hosts not written: %q", data)
	}

	// second time: host is known, only the password is asked
	ask = &scripted{answers: []string{"secret"}}
	c, err = dial(t, s, ask, hopFor(srv, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if len(ask.prompts) != 1 {
		t.Errorf("prompts on known host: %q", ask.prompts)
	}
}

func TestRejectUnknownHost(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"alice": "secret"}})
	s := testSettings(t)
	_, err := dial(t, s, &scripted{answers: []string{"no"}}, hopFor(srv, "alice"))
	if err == nil || !strings.Contains(err.Error(), "host key verification failed") {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(s.KnownHosts); len(data) != 0 {
		t.Errorf("known_hosts written after 'no': %q", data)
	}
}

func TestChangedHostKeyIsRefused(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"alice": "secret"}})
	s := testSettings(t)
	// put a different ed25519 key on file for this address
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	other, _ := ssh.NewPublicKey(pub)
	line := "[127.0.0.1]:" + strings.Split(srv.Addr, ":")[1] + " " + string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(other)))
	os.WriteFile(s.KnownHosts, []byte(line+"\n"), 0o600)

	ask := &scripted{answers: []string{"yes", "secret"}}
	_, err := dial(t, s, ask, hopFor(srv, "alice"))
	if err == nil || !strings.Contains(err.Error(), "HAS CHANGED") {
		t.Fatalf("err = %v", err)
	}
	if len(ask.prompts) != 0 {
		t.Errorf("must not prompt on a changed key: %q", ask.prompts)
	}
}

func writeKey(t *testing.T, passphrase string) (string, ssh.PublicKey) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	var block *pem.Block
	var err error
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_test")
	os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
	sp, _ := ssh.NewPublicKey(pub)
	return path, sp
}

func TestKeyFileAndPassphrase(t *testing.T) {
	plain, plainPub := writeKey(t, "")
	enc, encPub := writeKey(t, "hunter2")
	srv := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{plainPub, encPub}})
	s := testSettings(t)

	h := hopFor(srv, "bob")
	h.Identity = plain
	ask := &scripted{answers: []string{"yes"}}
	c, err := dial(t, s, ask, h)
	if err != nil {
		t.Fatalf("plain key: %v", err)
	}
	c.Close()

	// encrypted key from the defaults list: wrong passphrase, then right one
	s.Identities = []string{enc}
	h.Identity = ""
	ask = &scripted{answers: []string{"wrong", "hunter2"}}
	c, err = dial(t, s, ask, h)
	if err != nil {
		t.Fatalf("encrypted key: %v (prompts %q, infos %q)", err, ask.prompts, ask.infos)
	}
	c.Close()
	if len(ask.prompts) != 2 || !strings.Contains(ask.prompts[0], "passphrase") {
		t.Errorf("prompts = %q", ask.prompts)
	}
	if got := srv.LoginList(); len(got) != 2 || got[1] != "bob/publickey" {
		t.Errorf("logins = %q", got)
	}
}

func TestJumpHostAndShell(t *testing.T) {
	jump := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"j": "jp"}})
	target := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"t": "tp"}})
	s := testSettings(t)

	ask := &scripted{answers: []string{"yes", "jp", "yes", "tp"}}
	c, err := dial(t, s, ask, hopFor(jump, "j"), hopFor(target, "t"))
	if err != nil {
		t.Fatalf("dial: %v (prompts %q)", err, ask.prompts)
	}
	defer c.Close()
	if got := target.LoginList(); len(got) != 1 || got[0] != "t/password" {
		t.Errorf("target logins = %q", got)
	}
	if !strings.Contains(strings.Join(ask.infos, "\n"), "via j-srv") {
		t.Errorf("infos = %q", ask.infos)
	}

	var out syncBuffer
	sh, err := c.StartShell("xterm-256color", 100, 30, "", &out)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, &out, "welcome xterm-256color 100x30")
	sh.Write([]byte("hi\r"))
	waitFor(t, &out, "you said: hi")
	if err := sh.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	sh.Write([]byte("exit\r"))
	if err := sh.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	sizes := target.SizeList()
	if len(sizes) != 2 || sizes[1].Width != 120 || sizes[1].Height != 40 {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestCancelDuringPrompt(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"alice": "secret"}})
	s := testSettings(t)
	// no answers: the prompter cancels, which must fail the dial cleanly
	_, err := dial(t, s, &scripted{}, hopFor(srv, "alice"))
	if err == nil {
		t.Fatal("expected error")
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitFor(t *testing.T, b *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(b.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q; got %q", want, b.String())
}
