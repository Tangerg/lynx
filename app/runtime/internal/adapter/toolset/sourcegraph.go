package toolset

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

type sourcegraphConfig struct {
	Endpoint string
	Token    string
}

type sourcegraphRequest struct {
	Query        string `json:"query" jsonschema:"required" jsonschema_description:"Sourcegraph search query. Use repo:, file:, lang:, type:, patternType:, count:, etc. as needed."`
	MaxResults   int    `json:"max_results,omitempty" jsonschema_description:"Maximum matches to return. Default 10, max 50."`
	PatternType  string `json:"pattern_type,omitempty" jsonschema_description:"Optional pattern type when query does not already contain patternType:. One of standard, keyword, regexp."`
	ContextLines int    `json:"context_lines,omitempty" jsonschema_description:"Context lines around content matches. Default 1, max 5."`
}

type sourcegraphResponse struct {
	Matches    []sourcegraphMatch `json:"matches"`
	MatchCount int                `json:"match_count,omitempty"`
	DurationMs int                `json:"duration_ms,omitempty"`
	Alerts     []string           `json:"alerts,omitempty"`
}

type sourcegraphMatch struct {
	Type        string                 `json:"type"`
	Repository  string                 `json:"repository,omitempty"`
	Path        string                 `json:"path,omitempty"`
	Commit      string                 `json:"commit,omitempty"`
	Language    string                 `json:"language,omitempty"`
	LineMatches []sourcegraphLineMatch `json:"lineMatches,omitempty"`
}

type sourcegraphLineMatch struct {
	Line       string `json:"line"`
	LineNumber int    `json:"lineNumber"`
}

var sourcegraphSchema, _ = pkgjson.StringDefSchemaOf(sourcegraphRequest{})

type sourcegraphTool struct {
	streamURL string
	token     string
	client    *http.Client
}

func newSourcegraphTool(cfg sourcegraphConfig) (chat.Tool, error) {
	streamURL, err := sourcegraphStreamURL(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	return &sourcegraphTool{streamURL: streamURL, token: cfg.Token, client: http.DefaultClient}, nil
}

func (t *sourcegraphTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "sourcegraph_search",
		Description: "Search code with Sourcegraph streaming search. Use for public or configured Sourcegraph code search across repositories.",
		InputSchema: sourcegraphSchema,
	}
}

func (t *sourcegraphTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *sourcegraphTool) Call(ctx context.Context, arguments string) (string, error) {
	var req sourcegraphRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("sourcegraph_search: parse arguments: %w", err)
	}
	if strings.TrimSpace(req.Query) == "" {
		return "", errors.New("sourcegraph_search: query must not be empty")
	}
	out, err := t.search(ctx, req)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("sourcegraph_search: marshal: %w", err)
	}
	return string(body), nil
}

func (t *sourcegraphTool) search(ctx context.Context, in sourcegraphRequest) (sourcegraphResponse, error) {
	u, err := url.Parse(t.streamURL)
	if err != nil {
		return sourcegraphResponse{}, err
	}
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 50 {
		maxResults = 50
	}
	contextLines := in.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 5 {
		contextLines = 5
	}
	q := u.Query()
	q.Set("q", in.Query)
	q.Set("v", "V3")
	q.Set("display", fmt.Sprint(maxResults))
	q.Set("cm", "true")
	q.Set("cl", fmt.Sprint(contextLines))
	if in.PatternType != "" {
		q.Set("t", in.PatternType)
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return sourcegraphResponse{}, fmt.Errorf("sourcegraph_search: build request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	if t.token != "" {
		httpReq.Header.Set("Authorization", "token "+t.token)
	}
	res, err := t.client.Do(httpReq)
	if err != nil {
		return sourcegraphResponse{}, fmt.Errorf("sourcegraph_search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return sourcegraphResponse{}, fmt.Errorf("sourcegraph_search: %s", res.Status)
	}
	return readSourcegraphStream(res.Body)
}

func sourcegraphStreamURL(endpoint string) (string, error) {
	if strings.TrimSpace(endpoint) == "" {
		return "", errors.New("sourcegraph_search: endpoint must not be empty")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("sourcegraph_search: invalid endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("sourcegraph_search: unsupported endpoint scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("sourcegraph_search: endpoint host must not be empty")
	}
	if !strings.HasSuffix(u.Path, "/.api/search/stream") {
		u.Path = strings.TrimRight(u.Path, "/") + "/.api/search/stream"
	}
	u.RawQuery = ""
	return u.String(), nil
}

func readSourcegraphStream(r io.Reader) (sourcegraphResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var out sourcegraphResponse
	var eventName string
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventName == "done" {
				break
			}
			foldSourcegraphEvent(eventName, data.String(), &out)
			eventName = ""
			data.Reset()
			continue
		}
		if value, ok := strings.CutPrefix(line, "event:"); ok {
			eventName = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return sourcegraphResponse{}, fmt.Errorf("sourcegraph_search: read stream: %w", err)
	}
	return out, nil
}

func foldSourcegraphEvent(name, data string, out *sourcegraphResponse) {
	if strings.TrimSpace(data) == "" {
		return
	}
	switch name {
	case "matches":
		var matches []sourcegraphMatch
		if json.Unmarshal([]byte(data), &matches) == nil {
			out.Matches = append(out.Matches, matches...)
		}
	case "progress":
		var p struct {
			MatchCount int `json:"matchCount"`
			DurationMs int `json:"durationMs"`
		}
		if json.Unmarshal([]byte(data), &p) == nil {
			out.MatchCount = p.MatchCount
			out.DurationMs = p.DurationMs
		}
	case "alert":
		var alert struct {
			Title   string `json:"title"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(data), &alert) == nil {
			msg := strings.TrimSpace(alert.Message)
			if title := strings.TrimSpace(alert.Title); title != "" && msg != "" {
				msg = title + ": " + msg
			} else if title != "" {
				msg = title
			}
			if msg != "" {
				out.Alerts = append(out.Alerts, msg)
			}
		}
	}
}
