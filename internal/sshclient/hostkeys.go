package sshclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/holdemlab/sshdeck/internal/i18n"
)

// hostKeys verifies server keys against sshdeck's own known_hosts file and
// asks the user about unknown hosts (trust on first use).
type hostKeys struct {
	path string
	ask  Prompter
	mu   sync.Mutex // serialises reads/appends of the file
}

func newHostKeys(path string, ask Prompter) (*hostKeys, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	return &hostKeys{path: path, ask: ask}, nil
}

// known returns the keys on file for addr (nil if the host is unknown).
func (hk *hostKeys) known(addr string) ([]knownhosts.KnownKey, error) {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	cb, err := knownhosts.New(hk.path)
	if err != nil {
		return nil, err
	}
	// Probe with a throwaway key: the KeyError lists what is on file.
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	probe, _ := ssh.NewPublicKey(pub)
	err = cb(addr, &net.TCPAddr{}, probe)
	var ke *knownhosts.KeyError
	if errors.As(err, &ke) {
		return ke.Want, nil
	}
	return nil, nil
}

// algorithms limits the host key algorithms to the key types already on file,
// so a server offering several keys is checked against the one we know.
func (hk *hostKeys) algorithms(addr string) []string {
	keys, err := hk.known(addr)
	if err != nil || len(keys) == 0 {
		return nil
	}
	var algos []string
	seen := map[string]bool{}
	add := func(a string) {
		if !seen[a] {
			seen[a] = true
			algos = append(algos, a)
		}
	}
	for _, k := range keys {
		switch t := k.Key.Type(); t {
		case ssh.KeyAlgoRSA:
			add(ssh.KeyAlgoRSASHA512)
			add(ssh.KeyAlgoRSASHA256)
			add(ssh.KeyAlgoRSA)
		default:
			add(t)
		}
	}
	return algos
}

func (hk *hostKeys) callback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	hk.mu.Lock()
	cb, err := knownhosts.New(hk.path)
	if err == nil {
		err = cb(hostname, remote, key)
	}
	hk.mu.Unlock()
	if err == nil {
		return nil
	}

	var ke *knownhosts.KeyError
	if !errors.As(err, &ke) {
		return err
	}
	if len(ke.Want) > 0 {
		w := ke.Want[0]
		return errors.New(i18n.T("HOST KEY FOR %s HAS CHANGED (possible man-in-the-middle). "+
			"Server sent %s %s; %s:%d has %s %s. Remove that line if the change is expected",
			hostname, key.Type(), ssh.FingerprintSHA256(key), w.Filename, w.Line, w.Key.Type(), ssh.FingerprintSHA256(w.Key)))
	}

	answer, err := hk.ask.Ask(i18n.T(
		"The authenticity of host '%s' can't be established.\r\n"+
			"%s key fingerprint is %s.\r\n"+
			"Are you sure you want to continue connecting (yes/no)? ",
		hostname, strings.ToUpper(strings.TrimPrefix(key.Type(), "ssh-")), ssh.FingerprintSHA256(key)), true)
	if err != nil {
		return err
	}
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "yes" && a != "y" && a != "так" && a != "т" {
		return errors.New(i18n.T("host key verification failed"))
	}
	return hk.add(hostname, key)
}

func (hk *hostKeys) add(hostname string, key ssh.PublicKey) error {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	f, err := os.OpenFile(hk.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key))
	return err
}
