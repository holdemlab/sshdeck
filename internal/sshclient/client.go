// Package sshclient is sshdeck's built-in SSH client: it dials a chain of
// jump hosts, authenticates (agent, key files, password, keyboard-
// interactive), verifies host keys against its own known_hosts and opens an
// interactive pty session. It never reads ~/.ssh/config.
package sshclient

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/holdemlab/sshdeck/internal/config"

	"github.com/holdemlab/sshdeck/internal/i18n"
)

// Prompter is how the client talks to the user while connecting. In sshdeck
// it is the tab itself: messages and prompts are written into the terminal,
// answers are typed there.
type Prompter interface {
	// Ask shows prompt and returns the line the user typed. echo=false
	// hides the input (passwords).
	Ask(prompt string, echo bool) (string, error)
	// Info shows a progress message.
	Info(msg string)
}

// ErrCanceled is returned when the user aborts a prompt.
var ErrCanceled = errors.New("canceled")

// Conn is an established connection, possibly through jump hosts.
type Conn struct {
	Client  *ssh.Client
	clients []*ssh.Client
	stop    chan struct{}
	once    sync.Once
}

// Close closes the target connection and every jump host, last to first.
func (c *Conn) Close() error {
	c.once.Do(func() { close(c.stop) })
	var first error
	for i := len(c.clients) - 1; i >= 0; i-- {
		if err := c.clients[i].Close(); err != nil && first == nil && !errors.Is(err, net.ErrClosed) {
			first = err
		}
	}
	return first
}

// Dial connects through hops in order. The context cancels the attempt
// (including pending prompts if the prompter honours it).
func Dial(ctx context.Context, hops []Hop, s config.Settings, ask Prompter) (*Conn, error) {
	return DialWith(ctx, hops, s, ask, nil)
}

// DialWith is Dial with a password/key memory (nil: remember nothing).
func DialWith(ctx context.Context, hops []Hop, s config.Settings, ask Prompter, secrets *Secrets) (*Conn, error) {
	if len(hops) == 0 {
		return nil, errors.New(i18n.T("nothing to connect to"))
	}
	hk, err := newHostKeys(config.ExpandHome(s.KnownHosts), ask)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	auth := newAuth(s, ask)
	auth.sec = secrets
	defer auth.close()

	c := &Conn{stop: make(chan struct{})}
	var prev *ssh.Client
	for i, h := range hops {
		via := ""
		if prev != nil {
			via = " " + i18n.T("via") + " " + hops[i-1].Name
		}
		ask.Info(i18n.T("Connecting to %s (%s@%s)%s…", h.Name, h.User, h.Addr(), via))
		client, err := dialHop(ctx, prev, h, s, hk, auth)
		if err != nil {
			c.Close()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%s: %w", h.Name, err)
		}
		c.clients = append(c.clients, client)
		prev = client
	}
	c.Client = prev
	if s.Keepalive > 0 {
		go c.keepalive(s.Keepalive.Std())
	}
	return c, nil
}

func dialHop(ctx context.Context, prev *ssh.Client, h Hop, s config.Settings, hk *hostKeys, a *auth) (*ssh.Client, error) {
	addr := h.Addr()
	dctx, cancel := context.WithTimeout(ctx, s.ConnectTimeout.Std())
	var conn net.Conn
	var err error
	if prev == nil {
		var d net.Dialer
		conn, err = d.DialContext(dctx, "tcp", addr)
	} else {
		conn, err = prev.DialContext(dctx, "tcp", addr)
	}
	cancel()
	if err != nil {
		return nil, err
	}

	// Abort the handshake if the tab is closed. No fixed deadline here: the
	// user may be typing a password.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	methods, ha := a.methods(h)
	cfg := &ssh.ClientConfig{
		User:              h.User,
		Auth:              methods,
		HostKeyCallback:   hk.callback,
		HostKeyAlgorithms: hk.algorithms(addr),
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, cleanAuthError(err)
	}
	if ha.password != "" {
		a.sec.setPassword(ha.id, ha.password) // accepted: remember it
	}
	return ssh.NewClient(cc, chans, reqs), nil
}

func cleanAuthError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "unable to authenticate") {
		return errors.New(i18n.T("authentication failed (no key or password accepted)"))
	}
	if strings.Contains(strings.ToLower(msg), "too many authentication failures") {
		return errors.New(i18n.T("too many authentication failures: the server gave up before the right key " +
			"was offered — set identity for this session, or use_agent: false / fewer identities in settings"))
	}
	return err
}

func (c *Conn) keepalive(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			errc := make(chan error, 1)
			go func() {
				_, _, err := c.Client.SendRequest("keepalive@openssh.com", true, nil)
				errc <- err
			}()
			select {
			case err := <-errc:
				if err != nil {
					c.Close()
					return
				}
			case <-time.After(3 * every):
				c.Close() // server stopped answering
				return
			case <-c.stop:
				return
			}
		}
	}
}

// ---- authentication ----

type auth struct {
	s       config.Settings
	ask     Prompter
	agent   agent.ExtendedAgent
	agentNC net.Conn
	keys    map[string]*keyFile // key files by path, shared by all hops
	sec     *Secrets            // remembered passwords / keys (may be nil)
}

// hopAuth tracks the password typed for one hop, to remember it once the
// server accepts it.
type hopAuth struct {
	id         string // user@host:port
	password   string // last password sent
	triedSaved bool
}

// nextPassword returns the remembered password on the first try, then asks.
func (a *auth) nextPassword(ha *hopAuth, prompt string) (string, error) {
	if !ha.triedSaved {
		ha.triedSaved = true
		if pw, ok := a.sec.password(ha.id); ok {
			a.ask.Info(i18n.T("Using remembered password"))
			ha.password = pw
			return pw, nil
		}
	} else if ha.password != "" {
		if saved, ok := a.sec.password(ha.id); ok && saved == ha.password {
			a.sec.forgetPassword(ha.id)
			a.ask.Info(i18n.T("Remembered password was rejected."))
		}
	}
	pw, err := a.ask.Ask(prompt, false)
	if err != nil {
		return "", err
	}
	ha.password = pw
	return pw, nil
}

// isPasswordPrompt tells keyboard-interactive password questions apart from
// one-time codes, which must never be remembered.
func isPasswordPrompt(questions []string, echos []bool) bool {
	return len(questions) == 1 && !echos[0] && strings.Contains(strings.ToLower(questions[0]), "password")
}

func newAuth(s config.Settings, ask Prompter) *auth {
	a := &auth{s: s, ask: ask, keys: map[string]*keyFile{}}
	if sock := os.Getenv("SSH_AUTH_SOCK"); s.UseAgent && sock != "" {
		if nc, err := net.Dial("unix", sock); err == nil {
			a.agentNC = nc
			a.agent = agent.NewClient(nc)
		}
	}
	return a
}

func (a *auth) close() {
	if a.agentNC != nil {
		a.agentNC.Close()
	}
}

func (a *auth) methods(h Hop) ([]ssh.AuthMethod, *hopAuth) {
	ha := &hopAuth{id: h.User + "@" + h.Addr()}
	tries := 3
	if _, ok := a.sec.password(ha.id); ok {
		tries++ // one more for the remembered password
	}
	return []ssh.AuthMethod{
		ssh.PublicKeysCallback(func() ([]ssh.Signer, error) { return a.signers(h), nil }),
		ssh.RetryableAuthMethod(ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			if text := strings.TrimSpace(name + "\n" + instruction); text != "" {
				a.ask.Info(text)
			}
			if isPasswordPrompt(questions, echos) {
				pw, err := a.nextPassword(ha, questions[0])
				if err != nil {
					return nil, err
				}
				return []string{pw}, nil
			}
			answers := make([]string, len(questions))
			for i, q := range questions {
				ans, err := a.ask.Ask(q, echos[i])
				if err != nil {
					return nil, err
				}
				answers[i] = ans
			}
			return answers, nil
		}), tries),
		ssh.RetryableAuthMethod(ssh.PasswordCallback(func() (string, error) {
			return a.nextPassword(ha, i18n.T("%s@%s's password: ", h.User, h.Host))
		}), tries),
	}, ha
}

// signers lists the keys offered to the server, in this order:
//
//  1. the hop's own key (identity of the session or its folder),
//  2. keys from ssh-agent,
//  3. the default key files from the settings.
//
// Key files are not decrypted here: the server is first asked whether it
// accepts the public key, and only then is the private key loaded (asking
// for the passphrase if needed), like OpenSSH does.
func (a *auth) signers(h Hop) []ssh.Signer {
	var out []ssh.Signer
	seen := map[string]bool{} // public key blobs already offered

	add := func(s ssh.Signer) {
		k := string(s.PublicKey().Marshal())
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	explicit := config.ExpandHome(h.Identity)
	if explicit != "" {
		if kf := a.keyFile(explicit, true); kf != nil {
			add(kf)
		}
	}
	if a.agent != nil {
		if keys, err := a.agent.List(); err == nil {
			if ss, err := a.agent.Signers(); err == nil {
				for i, s := range ss {
					name := i18n.T("agent key")
					if i < len(keys) && keys[i].Comment != "" {
						name = i18n.T("agent key") + " " + keys[i].Comment
					}
					add(&namedSigner{Signer: s, name: name, ask: a.ask})
				}
			}
		}
	}
	for _, p := range a.s.Identities {
		p = config.ExpandHome(p)
		if p == explicit {
			continue
		}
		if kf := a.keyFile(p, false); kf != nil {
			add(kf)
		}
	}
	return out
}

// keyFile reads the public half of a key file. Encrypted keys stay
// encrypted until a server accepts them.
func (a *auth) keyFile(path string, explicit bool) *keyFile {
	if kf, ok := a.keys[path]; ok {
		return kf
	}
	kf, err := openKeyFile(path, a.ask)
	if kf != nil {
		kf.sec = a.sec
	}
	if err != nil {
		if explicit {
			a.ask.Info(i18n.T("Cannot use key %s: %v", tilde(path), err))
		}
		kf = nil
	}
	a.keys[path] = kf
	return kf
}

// keyFile is an ssh.Signer backed by a private key file that is decrypted
// lazily, on the first signature.
type keyFile struct {
	path string
	data []byte
	pub  ssh.PublicKey
	ask  Prompter

	signer ssh.Signer // decrypted key, once loaded
	failed error      // passphrase not given / wrong 3 times
	sec    *Secrets   // remembers the decrypted key for later connections
}

func openKeyFile(path string, ask Prompter) (*keyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	kf := &keyFile{path: path, data: data, ask: ask}
	signer, err := ssh.ParsePrivateKey(data)
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		kf.signer, kf.pub = signer, signer.PublicKey()
	case errors.As(err, &missing):
		kf.pub = missing.PublicKey // OpenSSH format stores it unencrypted
		if kf.pub == nil {
			// old PEM format: the public half is only in the .pub file
			pubData, perr := os.ReadFile(path + ".pub")
			if perr != nil {
				return nil, errors.New(i18n.T("encrypted key without public part (no %s.pub)", path))
			}
			if kf.pub, _, _, _, perr = ssh.ParseAuthorizedKey(pubData); perr != nil {
				return nil, fmt.Errorf("%s.pub: %w", path, perr)
			}
		}
	default:
		return nil, err
	}
	return kf, nil
}

func (k *keyFile) PublicKey() ssh.PublicKey { return k.pub }

// load decrypts the key, asking for the passphrase (up to 3 times).
func (k *keyFile) load() (ssh.Signer, error) {
	if k.signer != nil {
		return k.signer, nil
	}
	if k.failed != nil {
		return nil, k.failed
	}
	if s, ok := k.sec.key(k.path, k.pub); ok {
		k.signer = s
		return s, nil
	}
	for tries := 0; tries < 3; tries++ {
		pass, err := k.ask.Ask(i18n.T("Enter passphrase for key '%s': ", tilde(k.path)), false)
		if err != nil {
			k.failed = err
			return nil, err
		}
		if pass == "" {
			break // skip this key
		}
		signer, err := ssh.ParsePrivateKeyWithPassphrase(k.data, []byte(pass))
		if err == nil {
			k.signer = signer
			k.sec.setKey(k.path, signer)
			return signer, nil
		}
		if !errors.Is(err, x509.IncorrectPasswordError) {
			k.failed = err
			return nil, err
		}
		k.ask.Info(i18n.T("Bad passphrase."))
	}
	k.failed = errors.New(i18n.T("no passphrase for %s", k.path))
	return nil, k.failed
}

func (k *keyFile) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	s, err := k.load()
	if err != nil {
		return nil, err
	}
	k.ask.Info(i18n.T("Using key %s", tilde(k.path)))
	return s.Sign(rand, data)
}

// SignWithAlgorithm lets the client pick rsa-sha2-256/512 for RSA keys.
func (k *keyFile) SignWithAlgorithm(rand io.Reader, data []byte, algorithm string) (*ssh.Signature, error) {
	s, err := k.load()
	if err != nil {
		return nil, err
	}
	k.ask.Info(i18n.T("Using key %s", tilde(k.path)))
	if as, ok := s.(ssh.AlgorithmSigner); ok {
		return as.SignWithAlgorithm(rand, data, algorithm)
	}
	return s.Sign(rand, data)
}

// tilde shortens the home directory to ~ for messages.
func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "/" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// namedSigner reports which agent key was used.
type namedSigner struct {
	ssh.Signer
	name string
	ask  Prompter
}

func (n *namedSigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	n.ask.Info(i18n.T("Using %s", n.name))
	return n.Signer.Sign(rand, data)
}

func (n *namedSigner) SignWithAlgorithm(rand io.Reader, data []byte, algorithm string) (*ssh.Signature, error) {
	n.ask.Info(i18n.T("Using %s", n.name))
	if as, ok := n.Signer.(ssh.AlgorithmSigner); ok {
		return as.SignWithAlgorithm(rand, data, algorithm)
	}
	return n.Signer.Sign(rand, data)
}

// ---- interactive shell ----

// Shell is an interactive session with a pty.
type Shell struct {
	sess  *ssh.Session
	stdin io.WriteCloser
}

// StartShell requests a pty of the given size and starts the login shell
// (or command, if not empty). Output is written to out, which must be safe
// for concurrent use.
func (c *Conn) StartShell(term string, width, height int, command string, out io.Writer) (*Shell, error) {
	sess, err := c.Client.NewSession()
	if err != nil {
		return nil, err
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 38400,
		ssh.TTY_OP_OSPEED: 38400,
	}
	if err := sess.RequestPty(term, height, width, modes); err != nil {
		sess.Close()
		return nil, fmt.Errorf("pty request: %w", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	sess.Stdout = out
	sess.Stderr = out
	if command != "" {
		err = sess.Start(command)
	} else {
		err = sess.Shell()
	}
	if err != nil {
		sess.Close()
		return nil, err
	}
	return &Shell{sess: sess, stdin: stdin}, nil
}

// Write sends input to the remote side.
func (s *Shell) Write(p []byte) (int, error) { return s.stdin.Write(p) }

// Resize tells the server about a new terminal size.
func (s *Shell) Resize(width, height int) error { return s.sess.WindowChange(height, width) }

// Wait blocks until the remote shell exits.
func (s *Shell) Wait() error { return s.sess.Wait() }

// Close ends the session.
func (s *Shell) Close() error { return s.sess.Close() }
