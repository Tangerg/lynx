// Package toolset composes Runtime-owned and integrated tools, applies their
// execution/path/presentation policies, and resolves model-facing manifests.
//
// Tool discovery exposes the model-facing search_tools meta-tool: a
// progressive-disclosure surface over a set of tools deliberately withheld from
// the initial manifest. The withheld
// tools stay resolvable in the Run's registry but are not advertised, so the
// prompt does not carry every deferred JSON schema every round. The model
// calls search_tools to find the ones it needs; each match is staged through a
// strategy-supplied [ToolAdvertiser] and becomes callable on the next safe model
// boundary.
package toolset

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
	"github.com/Tangerg/lynx/core/chat"
)

// defaultLimit caps how many tools one search returns (and promotes). Kept small
// so the model loads what it needs incrementally rather than re-flooding the
// manifest with a whole catalog.
const discoveryDefaultLimit = 5

// selectPrefix switches search_tools from keyword search to exact selection:
// query "select:a,b,c" loads those tools by name, no scoring.
const discoverySelectPrefix = "select:"

type discoveryArgs struct {
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Describe the capability you need, or use select:name1,name2 to load exact tool names. Prefix a keyword with + to require it."`
	Limit int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum keyword matches to load. Defaults to 5. Exact select: queries ignore this value."`
}

// searchableTool is one withheld tool with its precomputed match terms.
type discoverableTool struct {
	definition chat.ToolDefinition
	source     string   // runtime or MCP server identity, for grouping and fairness
	nameTerms  []string // tokenized qualified name
	nameLower  string
	descLower  string
}

type toolAdvertiserContextKey struct{}

// ToolAdvertiser stages exact deferred Tool names for later model visibility. It
// changes visibility only; the Resolver has already frozen executable authority.
type ToolAdvertiser func(names ...string) error

// WithToolAdvertiser binds the execution strategy's visibility callback to one
// Tool invocation. Toolset remains framework-neutral: the callback decides how
// a successful discovery result is committed by that strategy.
func WithToolAdvertiser(ctx context.Context, advertiser ToolAdvertiser) context.Context {
	if advertiser == nil {
		return ctx
	}
	return context.WithValue(ctx, toolAdvertiserContextKey{}, advertiser)
}

// Discovery is the search_tools meta-tool over a fixed set of withheld tools. It is
// built per Run from the resolver's complete deferred set, so its advertised
// catalog and promotable definitions never drift.
type Discovery struct {
	entries []discoverableTool
	byName  map[string]discoverableTool
	names   []string // deferred tool names, in stable source-then-name order
	inner   toolcontract.Tool
}

var _ toolcontract.Tool = (*Discovery)(nil)

// NewDiscovery builds a search_tools tool over withheld. It returns nil when withheld is
// empty so the caller simply omits the tool — there is nothing to search.
func NewDiscovery(withheld []toolcontract.Tool) (*Discovery, error) {
	if len(withheld) == 0 {
		return nil, nil
	}
	t := &Discovery{byName: make(map[string]discoverableTool, len(withheld))}
	for _, tool := range withheld {
		def := tool.Definition()
		e := discoverableTool{
			definition: def,
			source:     sourceOf(tool),
			nameTerms:  tokenize(def.Name),
			nameLower:  strings.ToLower(def.Name),
			descLower:  strings.ToLower(def.Description),
		}
		t.entries = append(t.entries, e)
		t.byName[def.Name] = e
	}
	// Stable order: source, then name — drives the round-robin rotation and the
	// catalog listed in the description.
	slices.SortFunc(t.entries, func(a, b discoverableTool) int {
		if a.source != b.source {
			return strings.Compare(a.source, b.source)
		}
		return strings.Compare(a.definition.Name, b.definition.Name)
	})
	t.names = make([]string, len(t.entries))
	for i, e := range t.entries {
		t.names[i] = e.definition.Name
	}
	inner, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name:        toolname.SearchTools,
			Description: t.buildDescription(),
		},
		t.search,
	)
	if err != nil {
		return nil, fmt.Errorf("discovery: build search_tools: %w", err)
	}
	t.inner = inner
	return t, nil
}

// DeferredToolNames exposes the exact executable names represented by this
// search surface. The Resolver remains the authority for visible/deferred sets.
func (t *Discovery) DeferredToolNames() []string {
	if t == nil {
		return nil
	}
	return slices.Clone(t.names)
}

func (t *Discovery) Definition() chat.ToolDefinition {
	return t.inner.Definition()
}

// buildDescription folds the "N tools available but not loaded" reminder into the
// tool the model always sees, listing names grouped by source so it has the
// vocabulary to search or select. Only names (never schemas) are listed — that is
// the whole point of deferral.
func (t *Discovery) buildDescription() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Load additional built-in or integration tools on demand. %d tool(s) are available but omitted from the initial tool list to keep it focused. ",
		len(t.entries))
	b.WriteString("Search by capability (query=\"...\") or load exact tools (query=\"select:name1,name2\"); matches become directly callable on your next step.\n\nNot loaded:")
	lastSource := ""
	first := true
	for _, e := range t.entries {
		if first || e.source != lastSource {
			first = false
			lastSource = e.source
			fmt.Fprintf(&b, "\n  [%s] ", e.source)
			b.WriteString(e.definition.Name)
			continue
		}
		b.WriteString(", ")
		b.WriteString(e.definition.Name)
	}
	return b.String()
}

func (t *Discovery) Call(ctx context.Context, arguments string) (string, error) {
	return t.inner.Call(ctx, arguments)
}

func (t *Discovery) search(ctx context.Context, args discoveryArgs) (string, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", ErrEmptyQuery
	}
	limit := args.Limit
	if limit <= 0 {
		limit = discoveryDefaultLimit
	}

	var matches []discoverableTool
	if rest, ok := strings.CutPrefix(query, discoverySelectPrefix); ok {
		matches = t.selectByName(rest)
	} else {
		matches = t.searchByKeyword(query, limit)
	}
	if len(matches) == 0 {
		return t.renderNoMatch(query), nil
	}

	advertise, ok := ctx.Value(toolAdvertiserContextKey{}).(ToolAdvertiser)
	if !ok || advertise == nil {
		return "", errors.New("search_tools: Tool advertisement is unavailable")
	}
	names := make([]string, len(matches))
	for index, match := range matches {
		names[index] = match.definition.Name
	}
	if err := advertise(names...); err != nil {
		return "", fmt.Errorf("search_tools: advertise matches: %w", err)
	}
	return t.renderMatches(matches), nil
}

// selectByName resolves an exact "select:a,b,c" list, preserving request order
// and dropping unknown names.
func (t *Discovery) selectByName(list string) []discoverableTool {
	var out []discoverableTool
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

type rankedTool struct {
	tool  discoverableTool
	score int
}

// searchByKeyword ranks the withheld tools against the query terms, then spreads
// the top results across servers (round-robin) so one large integration cannot
// starve the others out of the result window.
func (t *Discovery) searchByKeyword(query string, limit int) []discoverableTool {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}
	var hits []rankedTool
	for _, e := range t.entries {
		if s, ok := scoreEntry(terms, e); ok {
			hits = append(hits, rankedTool{tool: e, score: s})
		}
	}
	// Highest score first; stable by name so ties are deterministic.
	slices.SortStableFunc(hits, func(a, b rankedTool) int {
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		return strings.Compare(a.tool.definition.Name, b.tool.definition.Name)
	})
	return roundRobinBySource(hits, limit)
}

// scoreEntry weights a name-term match above a description-only match. A term
// prefixed with + is mandatory: if it matches nothing, the tool is excluded.
func scoreEntry(terms []string, e discoverableTool) (int, bool) {
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
		case strings.Contains(e.nameLower, term):
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

// roundRobinBySource draws from each source's ranked list in turn until limit
// is reached, so one large capability family cannot monopolize the result.
func roundRobinBySource(hits []rankedTool, limit int) []discoverableTool {
	if len(hits) == 0 {
		return nil
	}
	perSource := make(map[string][]discoverableTool)
	var order []string // first-seen source order (already score-then-name sorted)
	for _, h := range hits {
		if _, ok := perSource[h.tool.source]; !ok {
			order = append(order, h.tool.source)
		}
		perSource[h.tool.source] = append(perSource[h.tool.source], h.tool)
	}
	out := make([]discoverableTool, 0, min(limit, len(hits)))
	for len(out) < limit {
		progressed := false
		for _, source := range order {
			queue := perSource[source]
			if len(queue) == 0 {
				continue
			}
			out = append(out, queue[0])
			perSource[source] = queue[1:]
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

func (t *Discovery) renderMatches(matches []discoverableTool) string {
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

func (t *Discovery) renderNoMatch(query string) string {
	return fmt.Sprintf("No tools matched %q. %d tool(s) are available — try a broader keyword, or select:name to load one by exact name.", query, len(t.entries))
}

func sourceOf(tool toolcontract.Tool) string {
	if id, ok := tool.(mcpToolIdentity); ok {
		server, _ := id.MCPToolIdentity()
		if server != "" {
			return server
		}
	}
	return "built-in"
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
	if line, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(s)
}
