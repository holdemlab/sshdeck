// Package tunnel implements SSH port forwarding: local (-L), remote (-R) and
// dynamic SOCKS5 (-D), on top of an established SSH connection.
package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Kind is the forwarding direction.
type Kind byte

const (
	Local   Kind = 'L' // local port → destination reached from the server
	Remote  Kind = 'R' // server port → destination reached from this machine
	Dynamic Kind = 'D' // local SOCKS5 proxy, destinations reached from the server
)

// Spec describes one forward, in ssh's own notation:
//
//	L [bind:]port:host:hostport
//	R [bind:]port:host:hostport
//	D [bind:]port
//
// The bind address defaults to 127.0.0.1 (localhost only).
type Spec struct {
	Kind     Kind
	BindHost string
	BindPort int
	DestHost string
	DestPort int
}

// Parse reads a spec such as "L 8080:localhost:80" or "D 1080". The kind
// letter may be glued to the rest ("L8080:...") and defaults to L when the
// value has a destination.
func Parse(s string) (Spec, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Spec{}, fmt.Errorf("empty tunnel")
	}
	var sp Spec
	switch c := s[0]; c {
	case 'L', 'l', 'R', 'r', 'D', 'd':
		sp.Kind = Kind(c &^ 0x20) // upper case
		s = strings.TrimSpace(strings.TrimPrefix(s[1:], " "))
	default:
		sp.Kind = Local
	}
	parts, err := splitAddr(s)
	if err != nil {
		return Spec{}, err
	}
	switch sp.Kind {
	case Dynamic:
		switch len(parts) {
		case 1:
			sp.BindPort, err = port(parts[0])
		case 2:
			sp.BindHost = parts[0]
			sp.BindPort, err = port(parts[1])
		default:
			return Spec{}, fmt.Errorf("D wants [bind:]port, got %q", s)
		}
	default:
		switch len(parts) {
		case 3:
			sp.BindPort, err = port(parts[0])
			sp.DestHost = parts[1]
			if err == nil {
				sp.DestPort, err = port(parts[2])
			}
		case 4:
			sp.BindHost = parts[0]
			sp.BindPort, err = port(parts[1])
			sp.DestHost = parts[2]
			if err == nil {
				sp.DestPort, err = port(parts[3])
			}
		default:
			return Spec{}, fmt.Errorf("%c wants [bind:]port:host:hostport, got %q", sp.Kind, s)
		}
		if sp.DestHost == "" {
			return Spec{}, fmt.Errorf("missing destination host in %q", s)
		}
	}
	if err != nil {
		return Spec{}, err
	}
	if sp.BindHost == "" || sp.BindHost == "localhost" {
		sp.BindHost = "127.0.0.1"
	}
	if sp.BindHost == "*" {
		sp.BindHost = "0.0.0.0"
	}
	return sp, nil
}

// splitAddr splits on ':' while keeping [ipv6] parts together.
func splitAddr(s string) ([]string, error) {
	var parts []string
	for s != "" {
		if s[0] == '[' {
			end := strings.Index(s, "]")
			if end < 0 {
				return nil, fmt.Errorf("missing ] in %q", s)
			}
			parts = append(parts, s[1:end])
			s = strings.TrimPrefix(s[end+1:], ":")
			continue
		}
		i := strings.Index(s, ":")
		if i < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:i])
		s = s[i+1:]
	}
	return parts, nil
}

func port(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 65535 {
		return 0, fmt.Errorf("bad port %q", s)
	}
	return n, nil
}

// BindAddr is where the listener is opened.
func (s Spec) BindAddr() string { return net.JoinHostPort(s.BindHost, strconv.Itoa(s.BindPort)) }

// DestAddr is where connections go.
func (s Spec) DestAddr() string { return net.JoinHostPort(s.DestHost, strconv.Itoa(s.DestPort)) }

// String formats the spec in the notation Parse accepts.
func (s Spec) String() string {
	bind := strconv.Itoa(s.BindPort)
	if s.BindHost != "127.0.0.1" {
		bind = hostPart(s.BindHost) + ":" + bind
	}
	if s.Kind == Dynamic {
		return "D " + bind
	}
	return fmt.Sprintf("%c %s:%s:%d", s.Kind, bind, hostPart(s.DestHost), s.DestPort)
}

// Describe is a human-readable form: "127.0.0.1:8080 → db:5432".
func (s Spec) Describe() string {
	switch s.Kind {
	case Dynamic:
		return s.BindAddr() + " (SOCKS5)"
	case Remote:
		return "server " + s.BindAddr() + " → " + s.DestAddr()
	}
	return s.BindAddr() + " → " + s.DestAddr()
}

func hostPart(h string) string {
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}
