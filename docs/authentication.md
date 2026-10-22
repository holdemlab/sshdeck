# Authentication and known_hosts

sshdeck has a built-in SSH client (Go, `golang.org/x/crypto/ssh`). It does not
read `~/.ssh/config` and does not use `~/.ssh/known_hosts`.

## Authentication order

For each hop (both jump hosts and the target server), sshdeck tries methods in
this order:

1. **Public keys**, in this order:
   1. **the session key** (the session's own `identity` or one inherited from
      its folder); for a jump session, its own `identity`;
   2. keys from **ssh-agent** (`$SSH_AUTH_SOCK`), if `use_agent: true`;
   3. keys from the **`identities`** list in [settings](settings.md), one by
      one.

   The same key is never offered twice, even if it is both in a file and in
   the agent.
2. **keyboard-interactive**: the server's questions (for example, an OTP) are
   shown in the tab, up to 3 attempts.
3. **Password**: a `user@host's password:` prompt in the tab, up to 3 attempts.

If no method works, you get the error `authentication failed (no key or
password accepted)`.

**A key is decrypted only if the server accepted it.** The server first sees
only the public part of the key: for the OpenSSH format it is stored
unencrypted in the file, and for old PEM it is read from the `key.pub` file
next to it. So the passphrase is asked only for a key the server is actually
ready to accept. Other encrypted keys (for example, the default
`~/.ssh/id_ed25519`) are not opened and ask nothing.

**Which key logged you in** is shown in the tab while connecting:

```
Connecting to acme/dev/proxy (deploy@203.0.113.10:22)…
Using key ~/.ssh/acme/id_ed25519
Connecting to acme/dev/main (deploy@10.0.0.12:22) via acme/dev/proxy…
Using key ~/.ssh/acme/id_ed25519
```

For agent keys it says `Using agent key <key comment>`.

A key that cannot be used:
- if it is a file from the `identities` list, it is skipped silently;
- if it is an explicitly set `identity`, the tab shows a `Cannot use key …`
  message with the reason: file not found, format not recognized, or an
  encrypted PEM without `.pub`.

## Keys with a passphrase

If a key is encrypted and the server accepted it, the tab shows
`Enter passphrase for key '…':`.
- Wrong passphrase: `Bad passphrase.` and the prompt again, up to 3 attempts.
- An empty `enter` skips this key.
- The decrypted key is remembered until you quit sshdeck (if
  `remember_passwords` is on), so the passphrase is asked once per run. To
  avoid typing it even after a restart, add the key to ssh-agent (`ssh-add`).

## Password login

There is nothing to turn on. If there is no key or the server accepted none,
sshdeck asks for the password right in the tab: `user@host's password:`, up to
3 attempts. keyboard-interactive questions are shown the same way (for example,
`Password:` and a one-time code). The server must allow password login
(`PasswordAuthentication` or `KbdInteractiveAuthentication` in sshd).

To stop typing the password, install your key on the server:
[`ctrl+a k`](#installing-a-key-on-the-server-ctrla-k).

## Remembering passwords (in memory)

While sshdeck is open, it remembers:
- **passwords** the server accepted (separately for each `user@host:port`);
- **decrypted keys**, so the passphrase is asked once per run.

So reconnecting (`r`), a new tab to the same server (`ctrl+a d`) and going
through the same jump host a second time ask nothing. The tab then shows
`Using remembered password`.

Rules:
- **Nothing is written to disk.** Everything is forgotten when you quit
  sshdeck.
- **Only what was accepted is remembered.** A password is kept in memory only
  after a successful login. A wrong one is not remembered.
- **A stale password is forgotten at once.** If the server rejects a remembered
  password (for example, it was changed), sshdeck forgets it, prints
  `Remembered password was rejected.` and asks for a new one.
- **One-time codes (OTP) are never remembered.** Only a keyboard-interactive
  question that contains the word "password" is remembered.
- **Turned off with the setting** `remember_passwords: false` (or "Remember pw"
  on the settings screen, `ctrl+a ,`). Turning it off clears everything
  remembered at once.

## Installing a key on the server (`ctrl+a k`)

Does the same as `ssh-copy-id`: appends your public key to
`~/.ssh/authorized_keys` on the server. It works over the connection that is
already open, so you don't need the password a second time.

1. Connect to the server (with a password) and press `ctrl+a k`.
2. sshdeck looks for a key: first the session's `identity`, then keys from the
   `identities` list in settings.
   - **Key found:** you see its type, fingerprint and path. `y` installs it,
     `g` lets you create a new key instead.
   - **No key:** sshdeck offers to create a new ed25519 key in
     `~/.ssh/id_ed25519`; if that file already exists, in
     `~/.ssh/sshdeck_ed25519`. You can set a passphrase (twice, to check it).
     An empty field means a key without a passphrase. Existing files are never
     overwritten.
3. The key is appended on the server to `~/.ssh/authorized_keys`:
   - the `~/.ssh` directory and the file are created if missing (modes 700 and
     600);
   - existing keys stay;
   - if this key is already there, nothing changes, and sshdeck tells you so.
4. If the session does not use this key yet, sshdeck asks
   `Use … for <session> from now on?`. `y` sets it as the session's Key file in
   `sessions.yaml`.

The next connection uses the key, with no password. If the key has a
passphrase, it is asked once per run (see above).

Limitations:
- the key is installed only on the tab's **target** server; for a jump host,
  install it separately by opening it as a session;
- the server needs a POSIX shell (`sh`): any Linux, BSD or macOS works.

## What is stored on disk

- **Passwords and passphrases are never written to disk**: only in memory,
  while sshdeck runs.
- Keys are not copied; sshdeck only reads files at the given paths. New files
  are created only on request (`ctrl+a k` → new key).
- Agent forwarding (`ssh -A`) is not supported yet.

## Typing in the tab

All prompts appear right in the tab's terminal, as in regular ssh:
- `enter` — confirm;
- `backspace` — delete a character;
- `ctrl+u` — delete the line;
- `ctrl+c` / `esc` — cancel the connection.

The password is not shown as you type. You can also paste from the clipboard.

## known_hosts

sshdeck keeps its own file of server keys: `~/.config/sshdeck/known_hosts`
(you can change the path in settings). The format is the same as OpenSSH: host,
key type, key. For port 22 it writes `host`, for others `[host]:port`. The host
is written the way it is set in the session: a name or an IP.

### First connection

```
The authenticity of host '10.0.10.11:22' can't be established.
ED25519 key fingerprint is SHA256:89Ii89/ueWZFHKZgw4tLQLUPIBeZD57G1kNuGqCttW….
Are you sure you want to continue connecting (yes/no)?
```

- `yes` (or `y`) — the key is written to the file, and the connection goes on.
- Anything else — the connection is aborted (`host key verification failed`).

It is best to check the fingerprint with the server's administrator or against
the output of `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub` on the server
itself.

For a jump chain, the fingerprint is asked for each new hop separately.

### Server key changed

If the saved key does not match the one the server sent, sshdeck **does not
connect and asks nothing**:

```
HOST KEY FOR 10.0.10.11:22 HAS CHANGED (possible man-in-the-middle).
Server sent ssh-ed25519 SHA256:…; /home/you/.config/sshdeck/known_hosts:3 has …
Remove that line if the change is expected
```

If the change is expected (the server was reinstalled), delete the given line
from `known_hosts`, reconnect (`r`) and accept the new key.

## Keepalive and dropped connections

- Every `keepalive` (30 s by default), sshdeck sends the server a keepalive
  request.
- If the server does not answer for longer than 3 × keepalive, the connection
  is closed.
- The tab shows `connection lost`; `r` reconnects.
- `keepalive: 0` turns this off.

The connect timeout (`connect_timeout`, 15 s) counts separately for each hop and
covers only the TCP connection. While you type a password, the timer does not
run.
