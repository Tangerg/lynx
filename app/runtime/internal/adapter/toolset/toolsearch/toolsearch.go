// Package toolsearch exposes the model-facing search_tools meta-tool: a
// progressive-disclosure surface over a set of tools deliberately withheld from
// the initial manifest (in lyra, the connected MCP servers' tools). The withheld
// tools stay resolvable in the turn's registry but are not advertised, so the
// prompt does not carry every server's full JSON schema every round. The model
// calls search_tools to find the ones it needs; each match is promoted into the
// advertised toolset for the rest of the turn (via [toolloop.PromoteTools]) so it
// becomes directly callable on the next round.
//
// This is the app-side half of the T3 tool-search capability; the framework half
// (mid-loop promotion) lives in agent/toolloop.
package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools"
)

// defaultLimit caps how many tools one search returns (and promotes). Kept small
// so the model loads what it needs incrementally rather than re-flooding the
// manifest with a whole catalog.
const defaultLimit = 5

// selectPrefix switches search_tools from keyword search to exact selection:
// query "select:a,b,c" loads those tools by name, no scoring.
const selectPrefix = "select:"

type searchArgs struct {
	Query string `json:"query" jsonschema:"required" jsonschema_description:"Either a natural-language description of the capability you need (e.g. \"create a Linear issue\"), which returns the best-matching tools; or \"select:name1,name2\" to load specific tools by exact name. Prefix a term with + to require it."`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Max tools to return and load (default 5). Ignored for select:."`
}

var searchSchema, _ = pkgjson.StringDefSchemaOf(searchArgs{})

// entry is one searchable withheld tool with its precomputed match terms.
type entry struct {
	tool       tools.Tool
	definition chat.ToolDefinition
	server     string   // MCP server name, for round-robin fairness; "" if unknown
	nameTerms  []string // tokenized qualified name
	descLower  string
}

type mcpIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// Tool is the search_tools meta-tool over a fixed set of withheld tools. It is
// built per turn with the turn's live MCP tool set (see the resolver), so its
// advertised catalog and promotable set never drift.
type Tool struct {
	entries []entry
	byName  map[string]entry
	names   []string // deferred tool names, in stable server-then-name order
	desc    string   // precomputed model-facing description (immutable per instance)
}

var _ tools.Tool = (*Tool)(nil)

// New builds a search_tools tool over withheld. It returns nil when withheld is
// empty so the caller simply omits the tool — there is nothing to search.
func New(withheld []tools.Tool) *Tool {
	if len(withheld) == 0 {
		return nil
	}
	t := &Tool{byName: make(map[string]entry, len(withheld))}
	for _, tool := range withheld {
		def := tool.Definition()
		e := entry{
			tool:       tool,
			definition: def,
			server:     serverOf(tool),
			nameTerms:  tokenize(def.Name),
			descLower:  strings.ToLower(def.Description),
		}
		t.entries = append(t.entries, e)
		t.byName[def.Name] = e
	}
	// Stable order: server, then name — drives the round-robin rotation and the
	// catalog listed in the description.
	slices.SortFunc(t.entries, func(a, b entry) int {
		if a.server != b.server {
			return strings.Compare(a.server, b.server)
		}
		return strings.Compare(a.definition.Name, b.definition.Name)
	})
	t.names = make([]string, len(t.entries))
	for i, e := range t.entries {
		t.names[i] = e.definition.Name
	}
	t.desc = t.buildDescription()
	return t
}

// DeferredToolNames reports the withheld tools this search surface owns. The
// turn manifest projection reads it to exclude these from the initial advertised
// toolset while keeping them resolvable for promotion.
func (t *Tool) DeferredToolNames() []string {
	if t == nil {
		return nil
	}
	return slices.Clone(t.names)
}

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "search_tools",
		Description: t.desc,
		InputSchema: json.RawMessage(searchSchema),
	}
}

// buildDescription folds the "N tools available but not loaded" reminder into the
// tool the model always sees, listing names grouped by server so it has the
// vocabulary to search or select. Only names (never schemas) are listed — that is
// the whole point of deferral.
func (t *Tool) buildDescription() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Load additional tools on demand. %d tool(s) from your connected integrations are available but not loaded into your context to keep it small. ",
		len(t.entries))
	b.WriteString("Search by capability (query=\"...\") or load exact tools (query=\"select:name1,name2\"); matches become directly callable on your next step.\n\nNot loaded:")
	lastServer := "\x00"
	for _, e := range t.entries {
		if e.server != lastServer {
			lastServer = e.server
			if e.server != "" {
				fmt.Fprintf(&b, "\n  [%s] ", e.server)
			} else {
				b.WriteString("\n  ")
			}
			b.WriteString(e.definition.Name)
			continue
		}
		b.WriteString(", ")
		b.WriteString(e.definition.Name)
	}
	return b.String()
}

func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("toolsearch: parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", ErrEmptyQuery
	}
	limit := args.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	var matches []entry
	if rest, ok := strings.CutPrefix(query, selectPrefix); ok {
		matches = t.selectByName(rest)
	} else {
		matches = t.searchByKeyword(query, limit)
	}
	if len(matches) == 0 {
		return t.renderNoMatch(query), nil
	}

	defs := make([]chat.ToolDefinition, len(matches))
	for i, m := range matches {
		defs[i] = m.definition
	}
	// Advertise the matches for the rest of the turn. Outside a running loop this
	// is a no-op; the listing is still returned so the call is never useless.
	toolloop.PromoteTools(ctx, defs...)
	return t.renderMatches(matches), nil
}

// selectByName resolves an exact "select:a,b,c" list, preserving request order
// and dropping unknown names.
func (t *Tool) selectByName(list string) []entry {
	var out []entry
	seen := make(map[string]struct{})
	for name := range strings.SplitSeq(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		if e, ok := t.byName[name]; ok {
			seen[name] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}

type scored struct {
	entry entry
	score int
}

// searchByKeyword ranks the withheld tools against the query terms, then spreads
// the top results across servers (round-robin) so one large integration cannot
// starve the others out of the result window.
func (t *Tool) searchByKeyword(query string, limit int) []entry {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}
	var hits []scored
	for _, e := range t.entries {
		if s, ok := scoreEntry(terms, e); ok {
			hits = append(hits, scored{entry: e, score: s})
		}
	}
	// Highest score first; stable by name so ties are deterministic.
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].entry.definition.Name < hits[j].entry.definition.Name
	})
	return roundRobinByServer(hits, limit)
}

// scoreEntry weights a name-term match above a description-only match. A term
// prefixed with + is mandatory: if it matches nothing, the tool is excluded.
func scoreEntry(terms []string, e entry) (int, bool) {
	total := 0
	for _, term := range terms {
		required := strings.HasPrefix(term, "+")
		term = strings.TrimPrefix(term, "+")
		if term == "" {
			continue
		}
		s := 0
		switch {
		case slices.Contains(e.nameTerms, term):
			s = 3
		case strings.Contains(strings.ToLower(e.definition.Name), term):
			s = 2
		case strings.Contains(e.descLower, term):
			s = 1
		}
		if s == 0 && required {
			return 0, false
		}
		total += s
	}
	return total, total > 0
}

// roundRobinByServer draws from each server's ranked list in turn until limit is
// reached, so the window is shared fairly across integrations while each server
// still contributes its best matches first.
func roundRobinByServer(hits []scored, limit int) []entry {
	if len(hits) == 0 {
		return nil
	}
	perServer := make(map[string][]entry)
	var order []string // first-seen server order (already score-then-name sorted)
	for _, h := range hits {
		if _, ok := perServer[h.entry.server]; !ok {
			order = append(order, h.entry.server)
		}
		perServer[h.entry.server] = append(perServer[h.entry.server], h.entry)
	}
	out := make([]entry, 0, min(limit, len(hits)))
	for len(out) < limit {
		progressed := false
		for _, server := range order {
			queue := perServer[server]
			if len(queue) == 0 {
				continue
			}
			out = append(out, queue[0])
			perServer[server] = queue[1:]
			progressed = true
			if len(out) >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	return out
}

func (t *Tool) renderMatches(matches []entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Loaded %d tool(s) — now callable directly on your next step:\n", len(matches))
	for _, m := range matches {
		b.WriteString("  - ")
		b.WriteString(m.definition.Name)
		if desc := firstLine(m.definition.Description); desc != "" {
			b.WriteString(": ")
			b.WriteString(desc)
		}
		b.WriteByte('\n')
	}
	if remaining := len(t.entries) - len(matches); remaining > 0 {
		fmt.Fprintf(&b, "%d other tool(s) remain unloaded — search again to load more.", remaining)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (t *Tool) renderNoMatch(query string) string {
	return fmt.Sprintf("No tools matched %q. %d tool(s) are available — try a broader keyword, or select:name to load one by exact name.", query, len(t.entries))
}

func serverOf(tool tools.Tool) string {
	if id, ok := tool.(mcpIdentity); ok {
		server, _ := id.MCPToolIdentity()
		return server
	}
	return ""
}

// tokenize splits a qualified tool name into lowercase terms on non-alphanumeric
// boundaries and camelCase humps, so "linear_create_issue" and "createIssue"
// both yield useful search terms.
func tokenize(name string) []string {
	var terms []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			terms = append(terms, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			flush()
		case i > 0 && isUpper(r) && !isUpper(runes[i-1]):
			flush()
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return terms
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
