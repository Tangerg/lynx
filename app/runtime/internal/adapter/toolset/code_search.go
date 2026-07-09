package toolset

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools/fs"
)

const defaultCodeSearchLimit = 8
const maxCodeSearchLimit = 20

type codeSearchRequest struct {
	Query            string `json:"query,omitempty" jsonschema_description:"Natural-language concept or behavior to find semantically in the current workspace."`
	Literal          string `json:"literal,omitempty" jsonschema_description:"Exact string or symbol to grep locally. Treated literally, not as a regex."`
	Glob             string `json:"glob,omitempty" jsonschema_description:"Optional local file glob for literal search, e.g. **/*.go."`
	SourcegraphQuery string `json:"sourcegraph_query,omitempty" jsonschema_description:"Optional Sourcegraph query. Runs only when Sourcegraph is configured; use Sourcegraph syntax such as repo:, file:, lang:, type:."`
	Limit            int    `json:"limit,omitempty" jsonschema_description:"Maximum results per source. Default 8, max 20."`
}

type codeSearchResponse struct {
	Semantic      []codeSearchSemanticHit `json:"semantic,omitempty"`
	Local         []codeSearchLocalMatch  `json:"local,omitempty"`
	Sourcegraph   []sourcegraphMatch      `json:"sourcegraph,omitempty"`
	SuggestedRead []codeSearchRead        `json:"suggested_reads,omitempty"`
	Notes         []string                `json:"notes,omitempty"`
}

type codeSearchSemanticHit struct {
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
}

type codeSearchLocalMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type codeSearchRead struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Source string `json:"source"`
}

var codeSearchSchema, _ = pkgjson.StringDefSchemaOf(codeSearchRequest{})

type codeSearchTool struct {
	workdir     string
	index       CodebaseIndex
	sourcegraph *sourcegraphTool
}

func newCodeSearchTool(workdir string, index CodebaseIndex, sourcegraph sourcegraphConfig) (chat.Tool, error) {
	var sg *sourcegraphTool
	if sourcegraph.enabled() {
		client, err := newSourcegraphClient(sourcegraph)
		if err != nil {
			return nil, err
		}
		sg = client
	}
	return &codeSearchTool{workdir: workdir, index: index, sourcegraph: sg}, nil
}

func (t *codeSearchTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "code_search",
		Description: "Plan code discovery across local semantic search, literal grep, and optional Sourcegraph. Returns ranked snippets and suggested file paths to read next; use read for full file context.",
		InputSchema: codeSearchSchema,
	}
}

func (t *codeSearchTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *codeSearchTool) Call(ctx context.Context, arguments string) (string, error) {
	var req codeSearchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("code_search: parse arguments: %w", err)
	}
	req = req.normalize()
	if req.Query == "" && req.Literal == "" && req.SourcegraphQuery == "" {
		return "", errors.New("code_search: provide query, literal, or sourcegraph_query")
	}
	out, err := t.search(ctx, req)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("code_search: marshal: %w", err)
	}
	return string(body), nil
}

func (r codeSearchRequest) normalize() codeSearchRequest {
	r.Query = strings.TrimSpace(r.Query)
	r.Literal = strings.TrimSpace(r.Literal)
	r.Glob = strings.TrimSpace(r.Glob)
	r.SourcegraphQuery = strings.TrimSpace(r.SourcegraphQuery)
	if r.Limit <= 0 {
		r.Limit = defaultCodeSearchLimit
	}
	if r.Limit > maxCodeSearchLimit {
		r.Limit = maxCodeSearchLimit
	}
	return r
}

func (t *codeSearchTool) search(ctx context.Context, req codeSearchRequest) (codeSearchResponse, error) {
	var out codeSearchResponse
	if req.Query != "" {
		hits, note, err := t.semantic(ctx, req)
		if err != nil {
			return codeSearchResponse{}, err
		}
		out.Semantic = hits
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
	}
	if req.Literal != "" {
		matches, err := t.local(ctx, req)
		if err != nil {
			return codeSearchResponse{}, err
		}
		out.Local = matches
	}
	if req.SourcegraphQuery != "" {
		matches, note, err := t.remote(ctx, req)
		if err != nil {
			return codeSearchResponse{}, err
		}
		out.Sourcegraph = matches
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
	}
	out.SuggestedRead = suggestedReads(out.Semantic, out.Local)
	return out, nil
}

func (t *codeSearchTool) semantic(ctx context.Context, req codeSearchRequest) ([]codeSearchSemanticHit, string, error) {
	if t.index == nil {
		return nil, "semantic search unavailable: no codebase index is configured", nil
	}
	if !t.index.Available(ctx) {
		return nil, "semantic search unavailable: no embedding model is configured", nil
	}
	hits, err := t.index.Search(ctx, t.workdir, req.Query, req.Limit)
	if err != nil {
		if errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
			return nil, "semantic search unavailable: no embedding model is configured", nil
		}
		return nil, "", fmt.Errorf("code_search: semantic: %w", err)
	}
	out := make([]codeSearchSemanticHit, len(hits))
	for i, hit := range hits {
		out[i] = codeSearchSemanticHit{
			Path:      hit.Path,
			StartLine: hit.StartLine,
			EndLine:   hit.EndLine,
			Score:     hit.Score,
			Snippet:   searchSnippet(hit.Text),
		}
	}
	return out, "", nil
}

func (t *codeSearchTool) local(ctx context.Context, req codeSearchRequest) ([]codeSearchLocalMatch, error) {
	out, err := fs.NewLocalExecutor(t.workdir).Grep(ctx, fs.GrepInput{
		Pattern:    regexp.QuoteMeta(req.Literal),
		Glob:       req.Glob,
		OutputMode: fs.GrepOutputContent,
		MaxResults: req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("code_search: local grep: %w", err)
	}
	matches := make([]codeSearchLocalMatch, len(out.Matches))
	for i, match := range out.Matches {
		matches[i] = codeSearchLocalMatch{
			Path: match.Path,
			Line: match.Line,
			Text: match.Text,
		}
	}
	return matches, nil
}

func (t *codeSearchTool) remote(ctx context.Context, req codeSearchRequest) ([]sourcegraphMatch, string, error) {
	if t.sourcegraph == nil {
		return nil, "sourcegraph search skipped: no Sourcegraph endpoint is configured", nil
	}
	res, err := t.sourcegraph.search(ctx, sourcegraphRequest{
		Query:        req.SourcegraphQuery,
		MaxResults:   req.Limit,
		ContextLines: 1,
	})
	if err != nil {
		return nil, "", fmt.Errorf("code_search: sourcegraph: %w", err)
	}
	notes := res.Alerts
	return res.Matches, strings.Join(notes, "; "), nil
}

func suggestedReads(semantic []codeSearchSemanticHit, local []codeSearchLocalMatch) []codeSearchRead {
	var out []codeSearchRead
	seen := map[codeSearchReadKey]int{}
	add := func(path string, line int, source string) {
		if path == "" {
			return
		}
		key := codeSearchReadKey{path: path, line: line}
		if i, ok := seen[key]; ok {
			out[i].Source = mergeReadSource(out[i].Source, source)
			return
		}
		seen[key] = len(out)
		out = append(out, codeSearchRead{Path: path, Line: line, Source: source})
	}
	for _, hit := range semantic {
		add(hit.Path, hit.StartLine, "semantic")
	}
	for _, match := range local {
		add(match.Path, match.Line, "literal")
	}
	slices.SortFunc(out, func(a, b codeSearchRead) int {
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		if a.Line != b.Line {
			return cmp.Compare(a.Line, b.Line)
		}
		return strings.Compare(a.Source, b.Source)
	})
	return out
}

type codeSearchReadKey struct {
	path string
	line int
}

func mergeReadSource(existing, next string) string {
	for source := range strings.SplitSeq(existing, ",") {
		if source == next {
			return existing
		}
	}
	if existing == "" {
		return next
	}
	return existing + "," + next
}

func searchSnippet(text string) string {
	const maxLines = 8
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n") + "\n..."
}
