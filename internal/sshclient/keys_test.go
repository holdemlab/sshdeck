package sshclient

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/holdemlab/sshdeck/internal/sshtest"
)

func countPrompts(prompts []string, sub string) int {
	n := 0
	for _, p := range prompts {
		if strings.Contains(p, sub) {
			n++
		}
	}
	return n
}

// The reported bug: the key from the session must be used, and other
// (encrypted) default keys must not be opened at all.
func TestSessionKeyFirstAndNoPassphraseForUnusedKeys(t *testing.T) {
	sessionKey, sessionPub := writeKey(t, "")
	otherEncrypted, _ := writeKey(t, "secret") // in identities, not accepted by the server
	srv := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{sessionPub}})
	s := testSettings(t)
	s.Identities = []string{otherEncrypted}

	h := hopFor(srv, "deploy")
	h.Identity = sessionKey
	ask := &scripted{answers: []string{"yes"}}
	c, err := dial(t, s, ask, h)
	if err != nil {
		t.Fatalf("dial: %v (prompts %q)", err, ask.prompts)
	}
	c.Close()
	if n := countPrompts(ask.prompts, "passphrase"); n != 0 {
		t.Errorf("asked for a passphrase of an unused key: %q", ask.prompts)
	}
	if !strings.Contains(strings.Join(ask.infos, "\n"), "Using key "+tilde(sessionKey)) {
		t.Errorf("infos do not say which key was used: %q", ask.infos)
	}
}

// An encrypted session key is decrypted only after the server accepted it,
// and the passphrase is asked once even with an encrypted default key around.
func TestEncryptedSessionKeyAskedOnce(t *testing.T) {
	sessionKey, sessionPub := writeKey(t, "pw1")
	otherEncrypted, _ := writeKey(t, "pw2")
	srv := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{sessionPub}})
	s := testSettings(t)
	s.Identities = []string{otherEncrypted}
	h := hopFor(srv, "u")
	h.Identity = sessionKey
	ask := &scripted{answers: []string{"yes", "pw1"}}
	c, err := dial(t, s, ask, h)
	if err != nil {
		t.Fatalf("dial: %v (prompts %q)", err, ask.prompts)
	}
	c.Close()
	if countPrompts(ask.prompts, "passphrase for key '"+tilde(sessionKey)+"'") != 1 || countPrompts(ask.prompts, otherEncrypted) != 0 {
		t.Errorf("prompts = %q", ask.prompts)
	}
}

// Many agent keys must not use up the server's MaxAuthTries before the
// session key is offered.
func TestSessionKeyBeforeAgentKeys(t *testing.T) {
	keyring := agent.NewKeyring()
	for i := 0; i < 8; i++ {
		_, priv, _ := ed25519.GenerateKey(rand.Reader)
		keyring.Add(agent.AddedKey{PrivateKey: priv, Comment: "unrelated"})
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, c)
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)

	sessionKey, sessionPub := writeKey(t, "")
	srv := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{sessionPub}})
	s := testSettings(t)
	s.UseAgent = true
	h := hopFor(srv, "u")
	h.Identity = sessionKey
	c, err := dial(t, s, &scripted{answers: []string{"yes"}}, h)
	if err != nil {
		t.Fatalf("session key not tried first: %v", err)
	}
	c.Close()

	// with that many agent keys and no session key, the server gives up;
	// the error explains what to do
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _ := ssh.NewPublicKey(other.Public())
	srvFull := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{otherPub}})
	_, err = dial(t, s, &scripted{answers: []string{"yes"}}, hopFor(srvFull, "u"))
	if err == nil || !strings.Contains(err.Error(), "use_agent") {
		t.Errorf("too many keys: err = %v", err)
	}

	// an agent key works when it is the accepted one
	keyring.RemoveAll()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	keyring.Add(agent.AddedKey{PrivateKey: priv, Comment: "work-laptop"})
	agentPub, _ := ssh.NewPublicKey(priv.Public())
	srv2 := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{agentPub}})
	ask := &scripted{answers: []string{"yes"}}
	c, err = dial(t, s, ask, hopFor(srv2, "u"))
	if err != nil {
		t.Fatalf("agent key: %v", err)
	}
	c.Close()
	if !strings.Contains(strings.Join(ask.infos, "\n"), "Using agent key work-laptop") {
		t.Errorf("infos = %q", ask.infos)
	}
}

// Old PEM-encrypted keys keep the public half only in the .pub file.
func TestLegacyEncryptedPEMUsesPubFile(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalECPrivateKey(priv)
	//nolint:staticcheck // legacy format on purpose
	block, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", der, []byte("old"), x509.PEMCipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_ecdsa")
	os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
	pub, _ := ssh.NewPublicKey(&priv.PublicKey)

	ask := &scripted{}
	if _, err := openKeyFile(path, ask); err == nil || !strings.Contains(err.Error(), ".pub") {
		t.Fatalf("without .pub: err = %v", err)
	}
	os.WriteFile(path+".pub", ssh.MarshalAuthorizedKey(pub), 0o644)

	srv := sshtest.Start(t, &sshtest.Server{AuthorizedKeys: []ssh.PublicKey{pub}})
	h := hopFor(srv, "u")
	h.Identity = path
	ask = &scripted{answers: []string{"yes", "old"}}
	c, err := dial(t, testSettings(t), ask, h)
	if err != nil {
		t.Fatalf("legacy key: %v (prompts %q)", err, ask.prompts)
	}
	c.Close()
}
