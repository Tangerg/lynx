package git

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Ignore matches repository-relative paths against .gitignore-style patterns.
// It loads the repo-root .gitignore plus .git/info/exclude — the subset a file
// watcher needs to prune ignored subtrees (build outputs, deps, caches) so a
// recursive watch doesn't burn a file descriptor on every generated file.
//
// Scope (a documented simplification, not full git semantics): only the
// repo-root rules are loaded, NOT nested per-directory .gitignore files, and
// the user's core.excludesFile is ignored. That's enough to skip the big
// ignored directories; anything missed merely gets watched (never wrongly
// hidden). Supported per-line syntax: comments (#), negation (!), root anchor
// (leading or mid slash), directory-only (trailing /), and * / ? / ** globs.
type Ignore struct {
	rules []ignoreRule
}

type ignoreRule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

// LoadIgnore reads root's ignore rules (root/.gitignore + root/.git/info/exclude).
// Always returns a usable (possibly empty) matcher — a missing file is not an
// error, so callers needn't nil-check.
func LoadIgnore(root string) *Ignore {
	ig := &Ignore{}
	for _, name := range []string{".gitignore", filepath.Join(".git", "info", "exclude")} {
		ig.addFile(filepath.Join(root, name))
	}
	return ig
}

func (ig *Ignore) addFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // best-effort: no such ignore file
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if r, ok := compileIgnoreRule(sc.Text()); ok {
			ig.rules = append(ig.rules, r)
		}
	}
}

// Match reports whether the repo-root-relative path (slash- or OS-separated) is
// ignored. isDir lets directory-only patterns (trailing /) apply. Last matching
// rule wins, so a later "!pat" can re-include an earlier match.
func (ig *Ignore) Match(rel string, isDir bool) bool {
	if ig == nil || len(ig.rules) == 0 {
		return false
	}
	rel = filepath.ToSlash(rel)
	ignored := false
	for _, r := range ig.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(rel) {
			ignored = !r.negate
		}
	}
	return ignored
}

// compileIgnoreRule turns one .gitignore line into a rule, or ok=false for a
// blank / comment line.
func compileIgnoreRule(line string) (ignoreRule, bool) {
	p := strings.TrimRight(line, " ")
	if p == "" || strings.HasPrefix(p, "#") {
		return ignoreRule{}, false
	}
	negate := strings.HasPrefix(p, "!")
	if negate {
		p = p[1:]
	}
	dirOnly := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return ignoreRule{}, false
	}
	// A separator anywhere but the (already-stripped) end anchors the pattern to
	// the repo root; otherwise it matches a basename at any depth.
	anchored := strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")

	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?") // basename match: at the root or any subdirectory
	}
	b.WriteString(globToRegex(p))
	b.WriteString("$")
	return ignoreRule{re: regexp.MustCompile(b.String()), negate: negate, dirOnly: dirOnly}, true
}

// globToRegex translates a gitignore glob body to a regexp body: * = any run
// within a path segment, ? = one such char, ** = across segments, everything
// else literal.
func globToRegex(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); {
		c := p[i]
		switch c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				if i+2 < len(p) && p[i+2] == '/' {
					b.WriteString("(?:.*/)?") // **/  → zero or more directories
					i += 3
					continue
				}
				b.WriteString(".*") // ** → anything, crossing separators
				i += 2
				continue
			}
			b.WriteString("[^/]*") // * → within one segment
			i++
		case '?':
			b.WriteString("[^/]")
			i++
		case '/':
			b.WriteByte('/')
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	return b.String()
}
