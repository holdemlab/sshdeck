package sshclient

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/holdemlab/sshdeck/internal/config"
)

// Hop is one SSH connection in a chain; the last hop is the target.
type Hop struct {
	Name     string // for messages, e.g. "prod/web-01" or "bastion"
	Host     string
	Port     int
	User     string
	Identity string
}

// Addr returns host:port.
func (h Hop) Addr() string {
	port := h.Port
	if port == 0 {
		port = 22
	}
	return joinHostPort(h.Host, port)
}

func joinHostPort(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return host + ":" + strconv.Itoa(port)
}

// Plan turns a resolved session into the list of hops to dial. Jump entries
// that name another session ("infra/proxy") use that session's settings,
// including its own jump; other entries are parsed as [user@]host[:port].
func Plan(cfg *config.Config, target config.Resolved) ([]Hop, error) {
	return plan(cfg, target, map[string]bool{})
}

func plan(cfg *config.Config, target config.Resolved, visiting map[string]bool) ([]Hop, error) {
	id := target.ID()
	if visiting[id] {
		return nil, fmt.Errorf("jump loop through %q", id)
	}
	visiting[id] = true
	defer delete(visiting, id)

	var hops []Hop
	for _, j := range strings.Split(target.Jump, ",") {
		j = strings.TrimSpace(j)
		if j == "" {
			continue
		}
		if ref, ok := cfg.Find(j); ok {
			sub, err := plan(cfg, ref, visiting)
			if err != nil {
				return nil, err
			}
			hops = append(hops, sub...)
			continue
		}
		h, err := ParseHop(j)
		if err != nil {
			return nil, fmt.Errorf("jump %q: %w", j, err)
		}
		if h.User == "" {
			h.User = target.User
		}
		if h.Identity == "" {
			h.Identity = target.Identity
		}
		hops = append(hops, h)
	}
	hops = append(hops, Hop{
		Name:     id,
		Host:     target.Host,
		Port:     target.Port,
		User:     target.User,
		Identity: target.Identity,
	})
	for i := range hops {
		if hops[i].User == "" {
			hops[i].User = localUser()
		}
	}
	return hops, nil
}

// ParseHop parses [user@]host[:port]; IPv6 hosts go in brackets.
func ParseHop(s string) (Hop, error) {
	var h Hop
	if i := strings.LastIndex(s, "@"); i >= 0 {
		h.User, s = s[:i], s[i+1:]
	}
	host, port := s, ""
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 {
			return h, fmt.Errorf("missing ]")
		}
		host = s[1:end]
		port = strings.TrimPrefix(s[end+1:], ":")
	} else if strings.Count(s, ":") == 1 {
		i := strings.Index(s, ":")
		host, port = s[:i], s[i+1:]
	}
	if host == "" {
		return h, fmt.Errorf("empty host")
	}
	h.Host, h.Name = host, s
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return h, fmt.Errorf("bad port %q", port)
		}
		h.Port = n
	}
	return h, nil
}

func localUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "root"
}
