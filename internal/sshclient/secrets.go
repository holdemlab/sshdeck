package sshclient

import (
	"sync"

	"golang.org/x/crypto/ssh"
)

// Secrets remembers passwords and decrypted keys in memory while sshdeck
// runs, so reconnecting, duplicating a tab or going through the same jump
// host does not ask again. Nothing is written to disk; everything is gone
// when the program exits. A nil *Secrets remembers nothing.
//
// A password is stored only after the server accepted it, and dropped as
// soon as the server rejects it.
type Secrets struct {
	mu        sync.Mutex
	passwords map[string]string     // "user@host:port" → password
	keys      map[string]ssh.Signer // key path + fingerprint → decrypted key
}

// NewSecrets returns an empty store.
func NewSecrets() *Secrets {
	return &Secrets{passwords: map[string]string{}, keys: map[string]ssh.Signer{}}
}

func (s *Secrets) password(id string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.passwords[id]
	return p, ok
}

func (s *Secrets) setPassword(id, pw string) {
	if s == nil || pw == "" {
		return
	}
	s.mu.Lock()
	s.passwords[id] = pw
	s.mu.Unlock()
}

func (s *Secrets) forgetPassword(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.passwords, id)
	s.mu.Unlock()
}

func keyID(path string, pub ssh.PublicKey) string {
	return path + "\x00" + ssh.FingerprintSHA256(pub)
}

func (s *Secrets) key(path string, pub ssh.PublicKey) (ssh.Signer, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[keyID(path, pub)]
	return k, ok
}

func (s *Secrets) setKey(path string, signer ssh.Signer) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.keys[keyID(path, signer.PublicKey())] = signer
	s.mu.Unlock()
}

// Clear forgets everything.
func (s *Secrets) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.passwords = map[string]string{}
	s.keys = map[string]ssh.Signer{}
	s.mu.Unlock()
}

// Count reports how many passwords and keys are remembered.
func (s *Secrets) Count() (passwords, keys int) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.passwords), len(s.keys)
}
