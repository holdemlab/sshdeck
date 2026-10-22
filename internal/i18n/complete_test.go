package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Every string the code passes to T (or to the helpers that translate their
// argument) must have a Ukrainian translation.

// translated lists calls whose argument at the given index is a message.
var translated = map[string]int{
	"T":         0, // ui.T and i18n.T
	"setStatus": 1,
}

// listVars are package-level tables of messages translated where shown.
var listVars = map[string]bool{
	"fieldLabels": true, "fieldHints": true, "languageLabels": true, "discardHint": true, "themeLabels": true,
}

func messages(t *testing.T) map[string]string {
	t.Helper()
	found := map[string]string{}
	dirs := []string{"../ui", "../sshclient", "../../cmd/sshdeck"}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			add := func(e ast.Expr) {
				for _, s := range literals(e) {
					found[s] = fset.Position(e.Pos()).String()
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					name := ""
					switch fn := n.Fun.(type) {
					case *ast.Ident:
						name = fn.Name
					case *ast.SelectorExpr:
						name = fn.Sel.Name
					}
					if i, ok := translated[name]; ok && i < len(n.Args) {
						add(n.Args[i])
					}
				case *ast.ValueSpec:
					for i, id := range n.Names {
						if listVars[id.Name] && i < len(n.Values) {
							ast.Inspect(n.Values[i], func(x ast.Node) bool {
								if kv, ok := x.(*ast.KeyValueExpr); ok {
									add(kv.Value)
									return false
								}
								if bl, ok := x.(*ast.BasicLit); ok {
									add(bl)
								}
								return true
							})
						}
					}
				case *ast.KeyValueExpr:
					// settings fields: {label: "...", help: "..."}
					if k, ok := n.Key.(*ast.Ident); ok && (k.Name == "label" || k.Name == "help") {
						add(n.Value)
					}
				}
				return true
			})
		}
	}
	return found
}

// literals returns the string constants in e, joining "a" + "b".
func literals(e ast.Expr) []string {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			if s, err := strconv.Unquote(e.Value); err == nil && s != "" {
				return []string{s}
			}
		}
	case *ast.BinaryExpr:
		l, r := literals(e.X), literals(e.Y)
		if len(l) == 1 && len(r) == 1 {
			if _, ok := e.X.(*ast.BasicLit); ok {
				if _, ok := e.Y.(*ast.BasicLit); ok {
					return []string{l[0] + r[0]}
				}
			}
			if _, ok := e.X.(*ast.BinaryExpr); ok {
				return []string{l[0] + r[0]}
			}
		}
	case *ast.ParenExpr:
		return literals(e.X)
	}
	return nil
}

func TestUkrainianComplete(t *testing.T) {
	found := messages(t)
	var missing []string
	for s, pos := range found {
		if strings.TrimSpace(s) == "" || !hasLetters(s) {
			continue
		}
		if _, ok := uk[s]; !ok {
			missing = append(missing, pos+": "+strconv.Quote(s))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		if out := os.Getenv("I18N_DUMP"); out != "" {
			var b strings.Builder
			keys := make([]string, 0, len(missing))
			for s := range found {
				if _, ok := uk[s]; !ok && hasLetters(s) {
					keys = append(keys, s)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString(strconv.Quote(k) + ": \"\",\n")
			}
			os.WriteFile(out, []byte(b.String()), 0o644)
		}
		t.Errorf("%d message(s) without a Ukrainian translation:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func hasLetters(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

// Translations keep the same fmt verbs in the same order.
func TestUkrainianVerbs(t *testing.T) {
	for en, ua := range uk {
		if verbs(en) != verbs(ua) {
			t.Errorf("verbs differ:\n  %q (%s)\n  %q (%s)", en, verbs(en), ua, verbs(ua))
		}
	}
}

func verbs(s string) string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		j := i + 1
		for j < len(s) && strings.IndexByte("0123456789.+-# []", s[j]) >= 0 {
			j++
		}
		if j < len(s) {
			out = append(out, s[i:j+1])
		}
		i = j
	}
	return strings.Join(out, " ")
}

func TestResolve(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "uk_UA.UTF-8")
	if Resolve("auto") != UK || Resolve("") != UK || Resolve("en") != EN {
		t.Fatal("auto should follow LANG")
	}
	t.Setenv("LANG", "en_US.UTF-8")
	if Resolve("auto") != EN || Resolve("uk") != UK {
		t.Fatal("explicit setting wins")
	}
	Set(UK)
	defer Set(EN)
	if T("search…") == "search…" {
		t.Fatal("not translated")
	}
	if T("no such message %d", 3) != "no such message 3" {
		t.Fatal("fallback")
	}
}
