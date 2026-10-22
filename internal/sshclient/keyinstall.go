package sshclient

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/holdemlab/sshdeck/internal/config"

	"github.com/holdemlab/sshdeck/internal/i18n"
)

// PublicKey is a local key pair that can be installed on a server.
type PublicKey struct {
	Path string        // private key file
	Key  ssh.PublicKey // public half
	Line string        // authorized_keys line: "type base64 comment"
}

// Describe is "ED25519 SHA256:… (~/.ssh/id_ed25519)".
func (p PublicKey) Describe() string {
	return fmt.Sprintf("%s %s (%s)", strings.ToUpper(strings.TrimPrefix(p.Key.Type(), "ssh-")),
		ssh.FingerprintSHA256(p.Key), tilde(p.Path))
}

// ReadPublicKey reads the public half of the key pair at path (the private
// key file must exist, since it is needed to log in later). The comment is
// taken from path.pub when there is one.
func ReadPublicKey(path string) (PublicKey, error) {
	path = config.ExpandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return PublicKey{}, err
	}
	pk := PublicKey{Path: path}
	if pubData, err := os.ReadFile(path + ".pub"); err == nil {
		if key, comment, _, _, err := ssh.ParseAuthorizedKey(pubData); err == nil {
			pk.Key = key
			pk.Line = strings.TrimSpace(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " " + comment)
			return pk, nil
		}
	}
	signer, err := ssh.ParsePrivateKey(data)
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		pk.Key = signer.PublicKey()
	case errors.As(err, &missing) && missing.PublicKey != nil:
		pk.Key = missing.PublicKey
	default:
		return PublicKey{}, errors.New(i18n.T("cannot read the public key of %s: %v", tilde(path), err))
	}
	pk.Line = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk.Key))) + " " + defaultComment()
	return pk, nil
}

// FindPublicKey returns the first usable key pair among paths.
func FindPublicKey(paths ...string) (PublicKey, bool) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if pk, err := ReadPublicKey(p); err == nil {
			return pk, true
		}
	}
	return PublicKey{}, false
}

// NewKeyPath suggests where to create a new key: ~/.ssh/id_ed25519, or
// ~/.ssh/sshdeck_ed25519(_N) if that exists.
func NewKeyPath() string {
	dir := config.ExpandHome("~/.ssh")
	candidates := []string{"id_ed25519", "sshdeck_ed25519"}
	for i := 2; i < 100; i++ {
		candidates = append(candidates, fmt.Sprintf("sshdeck_ed25519_%d", i))
	}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
	return filepath.Join(dir, "sshdeck_ed25519_new")
}

// GenerateKey creates a new ed25519 key pair at path (and path.pub),
// encrypted with passphrase when it is not empty. It never overwrites.
func GenerateKey(path, passphrase string) (PublicKey, error) {
	path = config.ExpandHome(path)
	for _, p := range []string{path, path + ".pub"} {
		if _, err := os.Stat(p); err == nil {
			return PublicKey{}, errors.New(i18n.T("%s already exists", tilde(p)))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return PublicKey{}, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PublicKey{}, err
	}
	comment := defaultComment()
	var block *pem.Block
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, comment, []byte(passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(priv, comment)
	}
	if err != nil {
		return PublicKey{}, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return PublicKey{}, err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + comment
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return PublicKey{}, err
	}
	if err := os.WriteFile(path+".pub", []byte(line+"\n"), 0o644); err != nil {
		return PublicKey{}, err
	}
	return PublicKey{Path: path, Key: sshPub, Line: line}, nil
}

func defaultComment() string {
	name := "user"
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	return name + "@" + host
}

// installScript must not contain single quotes (see InstallKey).
//
// installScript appends the key read from stdin to ~/.ssh/authorized_keys
// unless it is already there, fixing permissions like ssh-copy-id does.
const installScript = `umask 077
mkdir -p "$HOME/.ssh" || exit 1
chmod 700 "$HOME/.ssh" 2>/dev/null
f="$HOME/.ssh/authorized_keys"
k=$(cat)
[ -n "$k" ] || { echo "empty key" >&2; exit 1; }
touch "$f" || exit 1
set -- $k
if grep -qF -- "$2" "$f"; then echo EXISTS; exit 0; fi
if [ -s "$f" ] && [ -n "$(tail -c 1 "$f")" ]; then echo >> "$f"; fi
printf "%s\n" "$k" >> "$f" || exit 1
chmod 600 "$f" 2>/dev/null
echo ADDED
`

// InstallKey adds pk to ~/.ssh/authorized_keys on the server this
// connection leads to. It reports false when the key was already there.
// The server needs a POSIX shell (any Linux/BSD/macOS server).
func (c *Conn) InstallKey(pk PublicKey) (added bool, err error) {
	sess, err := c.Client.NewSession()
	if err != nil {
		return false, err
	}
	defer sess.Close()
	var out, errOut bytes.Buffer
	sess.Stdin = strings.NewReader(pk.Line + "\n")
	sess.Stdout = &out
	sess.Stderr = &errOut
	// The script has no single quotes, so wrapping it in '…' works the same
	// in sh, bash, zsh and fish login shells.
	if err := sess.Run("sh -c '" + installScript + "'"); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, errors.New(i18n.T("installing the key failed: %s", msg))
	}
	switch strings.TrimSpace(out.String()) {
	case "ADDED":
		return true, nil
	case "EXISTS":
		return false, nil
	}
	return false, errors.New(i18n.T("unexpected answer from the server: %q", strings.TrimSpace(out.String()+errOut.String())))
}
