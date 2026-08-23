package agenttools

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Tangerg/lynx/agent/interaction"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
)

const (
	defaultToolSearchLimit  = 8
	maxToolSearchLimit      = 20
	maxToolDescriptionRunes = 400
)

type searchToolsRequest struct {
	Query  string   `json:"query,omitempty" jsonschema:"maxLength=500" jsonschema_description:"Keywords describing the capability you need. Use either query or select."`
	Select []string `json:"select,omitempty" jsonschema:"maxItems=20" jsonschema_description:"Exact tool names returned by a previous search. Selected tools become visible on the next model call. Use either select or query."`
	Limit  int      `json:"limit,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum search matches. Defaults to 8."`
}

type searchToolsResponse struct {
	Mode    string              `json:"mode"`
	Matches []toolDiscoveryItem `json:"matches,omitempty"`
	Loaded  []toolDiscoveryItem `json:"loaded,omitempty"`
}

type toolDiscoveryItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type toolDiscoveryEntry struct {
	item       toolDiscoveryItem
	searchable string
}

func newToolDiscovery(deferred []agentexec.ExecutableTool) (toolcontract.Tool, error) {
	entries := make([]toolDiscoveryEntry, 0, len(deferred))
	byName := make(map[string]toolDiscoveryItem, len(deferred))
	for _, binding := range deferred {
		if !binding.Deferred {
			return nil, errors.New("agenttools: discovery received a visible tool")
		}
		definition := binding.Tool.Definition()
		if _, duplicate := byName[definition.Name]; duplicate {
			return nil, fmt.Errorf("agenttools: duplicate deferred tool %q", definition.Name)
		}
		item := toolDiscoveryItem{
			Name: definition.Name,
			Description: boundedDescription(definition.Description),
		}
		byName[item.Name] = item
		entries = append(entries, toolDiscoveryEntry{
			item: item,
			searchable: strings.ToLower(
				definition.Name + " " + definition.Description,
			),
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].item.Name < entries[right].item.Name
	})

	return toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name: "search_tools",
			Description: "Discover optional tools without loading every schema into context. Search by capability keywords, then call again with exact names in select. Selected tools become available from the next model call onward; this never grants new authority.",
		},
		func(ctx context.Context, request searchToolsRequest) (searchToolsResponse, error) {
			query := strings.TrimSpace(request.Query)
			if query != "" && len(request.Select) > 0 {
				return searchToolsResponse{}, errors.New("search_tools: use query or select, not both")
			}
			if len(request.Select) > 0 {
				return loadTools(ctx, request.Select, byName)
			}
			if query == "" {
				return searchToolsResponse{}, errors.New("search_tools: query or select is required")
			}
			limit := request.Limit
			if limit == 0 {
				limit = defaultToolSearchLimit
			}
			return searchToolIndex(query, limit, entries), nil
		},
	)
}

func loadTools(
	ctx context.Context,
	names []string,
	byName map[string]toolDiscoveryItem,
) (searchToolsResponse, error) {
	selected := make([]string, 0, len(names))
	loaded := make([]toolDiscoveryItem, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" || strings.TrimSpace(name) != name {
			return searchToolsResponse{}, fmt.Errorf("search_tools: invalid exact tool name %q", name)
		}
		item, found := byName[name]
		if !found {
			return searchToolsResponse{}, fmt.Errorf("search_tools: unknown deferred tool %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, name)
		loaded = append(loaded, item)
	}
	if len(selected) == 0 {
		return searchToolsResponse{}, errors.New("search_tools: select must contain a tool name")
	}
	if err := interaction.AdvertiseTools(ctx, selected...); err != nil {
		return searchToolsResponse{}, fmt.Errorf("search_tools: load selected tools: %w", err)
	}
	return searchToolsResponse{Mode: "select", Loaded: loaded}, nil
}

func searchToolIndex(
	query string,
	limit int,
	entries []toolDiscoveryEntry,
) searchToolsResponse {
	type rankedEntry struct {
		entry toolDiscoveryEntry
		score int
	}
	terms := searchTerms(query)
	if len(terms) == 0 {
		return searchToolsResponse{Mode: "search"}
	}
	ranked := make([]rankedEntry, 0, len(entries))
	for _, entry := range entries {
		score, matched := toolSearchScore(query, terms, entry)
		if matched {
			ranked = append(ranked, rankedEntry{entry: entry, score: score})
		}
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].score != ranked[right].score {
			return ranked[left].score > ranked[right].score
		}
		return ranked[left].entry.item.Name < ranked[right].entry.item.Name
	})
	limit = min(limit, maxToolSearchLimit, len(ranked))
	matches := make([]toolDiscoveryItem, 0, limit)
	for _, value := range ranked[:limit] {
		matches = append(matches, value.entry.item)
	}
	return searchToolsResponse{Mode: "search", Matches: matches}
}

func toolSearchScore(
	query string,
	terms []string,
	entry toolDiscoveryEntry,
) (int, bool) {
	name := strings.ToLower(entry.item.Name)
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	score := 0
	switch {
	case name == normalizedQuery:
		score += 1000
	case strings.HasPrefix(name, normalizedQuery):
		score += 400
	case strings.Contains(name, normalizedQuery):
		score += 250
	case strings.Contains(entry.searchable, normalizedQuery):
		score += 100
	}
	for _, term := range terms {
		if !strings.Contains(entry.searchable, term) {
			return 0, false
		}
		switch {
		case name == term:
			score += 100
		case strings.Contains(name, term):
			score += 40
		default:
			score += 10
		}
	}
	return score, true
}

func searchTerms(query string) []string {
	values := strings.FieldsFunc(strings.ToLower(query), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsNumber(value) &&
			value != '_' && value != '-'
	})
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func boundedDescription(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxToolDescriptionRunes {
		return string(runes)
	}
	return string(runes[:maxToolDescriptionRunes-1]) + "…"
}
