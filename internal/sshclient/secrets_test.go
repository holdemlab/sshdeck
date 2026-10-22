package sshclient

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/sshtest"
)

func dialSecrets(t *testing.T, s config.Settings, ask *scripted, sec *Secrets, hops ...Hop) (*Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return DialWith(ctx, hops, s, ask, sec)
}

func TestRememberedPassword(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "right"}})
	s := testSettings(t)
	sec := NewSecrets()

	// wrong first, then right: only the accepted one is remembered
	ask := &scripted{answers: []string{"yes", "wrong", "right"}}
	c, err := dialSecrets(t, s, ask, sec, hopFor(srv, "u"))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if pw, ok := sec.password("u@" + srv.Addr); !ok || pw != "right" {
		t.Fatalf("remembered %q %v", pw, ok)
	}

	// second connection: no prompt at all
	ask = &scripted{}
	c, err = dialSecrets(t, s, ask, sec, hopFor(srv, "u"))
	if err != nil {
		t.Fatalf("with remembered password: %v (prompts %q)", err, ask.prompts)
	}
	c.Close()
	if len(ask.prompts) != 0 || !strings.Contains(strings.Join(ask.infos, "\n"), "Using remembered password") {
		t.Errorf("prompts %q infos %q", ask.prompts, ask.infos)
	}

	// without a store: asks every time
	ask = &scripted{answers: []string{"right"}}
	c, err = dialSecrets(t, s, ask, nil, hopFor(srv, "u"))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if len(ask.prompts) != 1 {
		t.Errorf("nil store prompts = %q", ask.prompts)
	}
}

func TestRejectedRememberedPasswordIsForgotten(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "new"}})
	s := testSettings(t)
	sec := NewSecrets()
	sec.setPassword("u@"+srv.Addr, "old") // password changed on the server

	ask := &scripted{answers: []string{"yes", "new"}}
	c, err := dialSecrets(t, s, ask, sec, hopFor(srv, "u"))
	if err != nil {
		t.Fatalf("%v (prompts %q)", err, ask.prompts)
	}
	c.Close()
	if !strings.Contains(strings.Join(ask.infos, "\n"), "Remembered password was rejected") {
		t.Errorf("infos = %q", ask.infos)
	}
	if pw, _ := sec.password("u@" + srv.Addr); pw != "new" {
		t.Errorf("store has %q, want the new password", pw)
	}
}

func TestKeyboardInteractivePasswordRememberedButNotOTP(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "pw"}, KeyboardInteractive: true, OTP: "123456"})
	s := testSettings(t)
	sec := NewSecrets()
	ask := &scripted{answers: []string{"yes", "pw", "123456"}}
	c, err := dialSecrets(t, s, ask, sec, hopFor(srv, "u"))
	if err != nil {
		t.Fatalf("%v (prompts %q)", err, ask.prompts)
	}
	c.Close()

	// next time: password from memory, the one-time code is asked again
	ask = &scripted{answers: []string{"123456"}}
	c, err = dialSecrets(t, s, ask, sec, hopFor(srv, "u"))
	if err != nil {
		t.Fatalf("second: %v (prompts %q)", err, ask.prompts)
	}
	c.Close()
	if len(ask.prompts) != 1 || !strings.Contains(ask.prompts[0], "Verification code") {
		t.Errorf("prompts = %q", ask.prompts)
	}
}

func TestPassphraseRemembered(t *testing.T) {
	key, pub := writeKey(t, "phrase")
	srv := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{pub}})
	s := testSettings(t)
	sec := NewSecrets()
	h := hopFor(srv, "u")
	h.Identity = key
	ask := &scripted{answers: []string{"yes", "phrase"}}
	c, err := dialSecrets(t, s, ask, sec, h)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	ask = &scripted{}
	c, err = dialSecrets(t, s, ask, sec, h)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	c.Close()
	if len(ask.prompts) != 0 {
		t.Errorf("passphrase asked again: %q", ask.prompts)
	}
	sec.Clear()
	if p, k := sec.Count(); p+k != 0 {
		t.Error("Clear left secrets behind")
	}
}

// shShell runs exec requests with a real sh, HOME pointing at home.
func shShell(home string) sshtest.ShellFunc {
	return func(ch ssh.Channel, _ sshtest.Pty, command string, _ <-chan sshtest.Pty) uint32 {
		cmd := exec.Command("sh", "-c", command)
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
		cmd.Stdin = ch
		cmd.Stdout = ch
		cmd.Stderr = ch.Stderr()
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return uint32(ee.ExitCode())
			}
			io.WriteString(ch.Stderr(), err.Error())
			return 1
		}
		return 0
	}
}

func TestInstallKey(t *testing.T) {
	home := t.TempDir()
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "pw"}, Shell: shShell(home)})
	s := testSettings(t)
	c, err := dial(t, s, &scripted{answers: []string{"yes", "pw"}}, hopFor(srv, "u"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// an existing authorized_keys without a trailing newline
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	ak := filepath.Join(home, ".ssh", "authorized_keys")
	os.WriteFile(ak, []byte("ssh-ed25519 AAAAexisting other@box"), 0o600)

	key := filepath.Join(t.TempDir(), "id_new")
	pk, err := GenerateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateKey(key, ""); err == nil {
		t.Error("GenerateKey overwrote an existing key")
	}
	added, err := c.InstallKey(pk)
	if err != nil || !added {
		t.Fatalf("install: %v %v", added, err)
	}
	added, err = c.InstallKey(pk)
	if err != nil || added {
		t.Fatalf("second install should report existing: %v %v", added, err)
	}
	data, _ := os.ReadFile(ak)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != "ssh-ed25519 AAAAexisting other@box" || lines[1] != pk.Line {
		t.Errorf("authorized_keys:\n%s", data)
	}
	if st, _ := os.Stat(ak); st.Mode().Perm() != 0o600 {
		t.Errorf("authorized_keys mode %v", st.Mode().Perm())
	}

	// and the installed key now logs in without a password
	srv.AuthorizedKeys = []ssh.PublicKey{pk.Key}
	h := hopFor(srv, "u")
	h.Identity = key
	ask := &scripted{}
	c2, err := dial(t, s, ask, h)
	if err != nil {
		t.Fatalf("login with the installed key: %v (prompts %q)", err, ask.prompts)
	}
	c2.Close()
}

func TestReadPublicKey(t *testing.T) {
	// encrypted OpenSSH key without .pub: public half from the file itself
	key, pub := writeKey(t, "x")
	pk, err := ReadPublicKey(key)
	if err != nil || string(pk.Key.Marshal()) != string(pub.Marshal()) {
		t.Fatalf("encrypted: %v", err)
	}
	// .pub comment is kept
	os.WriteFile(key+".pub", []byte(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))+" me@laptop\n"), 0o644)
	pk, _ = ReadPublicKey(key)
	if !strings.HasSuffix(pk.Line, " me@laptop") {
		t.Errorf("line = %q", pk.Line)
	}
	if _, ok := FindPublicKey("", filepath.Join(t.TempDir(), "missing"), key); !ok {
		t.Error("FindPublicKey skipped a usable key")
	}
	if _, ok := FindPublicKey(filepath.Join(t.TempDir(), "missing")); ok {
		t.Error("FindPublicKey found a missing key")
	}
}
