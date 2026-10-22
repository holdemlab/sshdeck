package ui

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/holdemlab/sshdeck/internal/config"
)

type nodeKind int

const (
	folderNode nodeKind = iota
	sessionNode
	localNode // the built-in "local" entry: a shell on this computer
)

// node is one visible row of the session tree.
type node struct {
	kind    nodeKind
	name    string
	depth   int
	key     string   // stable id, used for the collapsed set
	path    []string // folder: its own path; session: the folder it lives in
	session *config.Session
	count   int // folderNode: number of sessions below
}

// id is the full path of a session ("prod/web/web-01") or folder ("prod/web").
func (n node) id() string {
	if n.kind == folderNode {
		return strings.Join(n.path, "/") // a folder's path already ends with its name
	}
	return strings.Join(append(append([]string{}, n.path...), n.name), "/")
}

func (n node) hostString() string {
	if n.session != nil {
		return n.session.Host
	}
	return ""
}

// buildTree flattens the config into visible rows, skipping the children of
// collapsed folders.
func buildTree(cfg *config.Config, collapsed map[string]bool) []node {
	var out []node
	var walk func(path []string, folders []*config.Folder, sessions []*config.Session, depth int)
	walk = func(path []string, folders []*config.Folder, sessions []*config.Session, depth int) {
		for _, f := range folders {
			p := append(append([]string{}, path...), f.Name)
			key := "f:" + strings.Join(p, "/")
			out = append(out, node{kind: folderNode, name: f.Name, depth: depth, key: key, path: p, count: countSessions(f)})
			if !collapsed[key] {
				walk(p, f.Folders, f.Sessions, depth+1)
			}
		}
		for _, s := range sessions {
			n := node{kind: sessionNode, name: s.Name, depth: depth, session: s, path: append([]string{}, path...)}
			n.key = "s:" + n.id()
			out = append(out, n)
		}
	}
	walk(nil, cfg.Folders, cfg.Sessions, 0)
	return out
}

func countSessions(f *config.Folder) int {
	n := len(f.Sessions)
	for _, sub := range f.Folders {
		n += countSessions(sub)
	}
	return n
}

func totalSessions(cfg *config.Config) int {
	n := len(cfg.Sessions)
	for _, f := range cfg.Folders {
		n += countSessions(f)
	}
	return n
}

// allSessions returns every session row regardless of collapsed state.
func allSessions(cfg *config.Config) []node {
	var out []node
	for _, n := range buildTree(cfg, nil) {
		if n.kind == sessionNode {
			n.depth = 0
			out = append(out, n)
		}
	}
	return out
}

type nodeSource []node

func (s nodeSource) String(i int) string { return s[i].id() + " " + s[i].hostString() }
func (s nodeSource) Len() int            { return len(s) }

// filterNodes fuzzy-matches query against session paths and hosts.
func filterNodes(nodes []node, query string) []node {
	if strings.TrimSpace(query) == "" {
		return nodes
	}
	matches := fuzzy.FindFrom(query, nodeSource(nodes))
	sort.Stable(matches)
	out := make([]node, 0, len(matches))
	for _, m := range matches {
		out = append(out, nodes[m.Index])
	}
	return out
}
