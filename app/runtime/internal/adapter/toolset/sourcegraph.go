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

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools"
)

type sourcegraphConfig struct {
	Endpoint string
	Token    string
}

func (config sourcegraphConfig) enabled() bool {
	return strings.TrimSpace(config.Endpoint) != ""
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
	LineMatches []sourcegraphLineMatch `json:"line_matches,omitempty"`
}

type sourcegraphLineMatch struct {
	Line       string `json:"line"`
	LineNumber int    `json:"line_number"`
}

var sourcegraphSchema = pkgjson.MustStringDefSchemaOf(sourcegraphRequest{})

type sourcegraphTool struct {
	streamURL string
	token     string
	client    *http.Client
}

func newSourcegraphTool(config sourcegraphConfig) (tools.Tool, error) {
	streamURL, err := sourcegraphStreamURL(config.Endpoint)
	if err != nil {
		return nil, err
	}
	return &sourcegraphTool{streamURL: streamURL, token: config.Token, client: http.DefaultClient}, nil
}

func (t *sourcegraphTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "sourcegraph_search",
		Description: "Search code with Sourcegraph streaming search. Use for public or configured Sourcegraph code search across repositories.",
		InputSchema: json.RawMessage(sourcegraphSchema),
	}
}

func (t *sourcegraphTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *sourcegraphTool) Call(ctx context.Context, arguments string) (string, error) {
	var request sourcegraphRequest
	if err := json.Unmarshal([]byte(arguments), &request); err != nil {
		return "", fmt.Errorf("sourcegraph_search: parse arguments: %w", err)
	}
	if strings.TrimSpace(request.Query) == "" {
		return "", errors.New("sourcegraph_search: query must not be empty")
	}
	output, err := t.search(ctx, request)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(output)
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
	maxResults = min(maxResults, 50)
	contextLines := min(max(in.ContextLines, 0), 5)
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
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/.api/search/stream") {
		path += "/.api/search/stream"
	}
	u.Path = path
	u.RawQuery = ""
	return u.String(), nil
}

func readSourcegraphStream(r io.Reader) (sourcegraphResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var output sourcegraphResponse
	var eventName string
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventName == "done" {
				break
			}
			if err := foldSourcegraphEvent(eventName, data.String(), &output); err != nil {
				return sourcegraphResponse{}, err
			}
			eventName = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
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
			data.WriteString(strings.TrimPrefix(value, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return sourcegraphResponse{}, fmt.Errorf("sourcegraph_search: read stream: %w", err)
	}
	if eventName != "" && eventName != "done" {
		if err := foldSourcegraphEvent(eventName, data.String(), &output); err != nil {
			return sourcegraphResponse{}, err
		}
	}
	return output, nil
}

func foldSourcegraphEvent(name, data string, output *sourcegraphResponse) error {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	switch name {
	case "matches":
		var matches []sourcegraphRawMatch
		if err := json.Unmarshal([]byte(data), &matches); err != nil {
			return fmt.Errorf("sourcegraph_search: parse matches event: %w", err)
		}
		for _, match := range matches {
			output.Matches = append(output.Matches, match.view())
		}
	case "progress":
		var p struct {
			MatchCount int `json:"matchCount"`
			DurationMs int `json:"durationMs"`
		}
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			return fmt.Errorf("sourcegraph_search: parse progress event: %w", err)
		}
		output.MatchCount = p.MatchCount
		output.DurationMs = p.DurationMs
	case "alert":
		var alert struct {
			Title   string `json:"title"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &alert); err != nil {
			return fmt.Errorf("sourcegraph_search: parse alert event: %w", err)
		}
		msg := strings.TrimSpace(alert.Message)
		if title := strings.TrimSpace(alert.Title); title != "" && msg != "" {
			msg = title + ": " + msg
		} else if title != "" {
			msg = title
		}
		if msg != "" {
			output.Alerts = append(output.Alerts, msg)
		}
	}
	return nil
}

type sourcegraphRawMatch struct {
	Type        string                    `json:"type"`
	Repository  string                    `json:"repository,omitempty"`
	Path        string                    `json:"path,omitempty"`
	Commit      string                    `json:"commit,omitempty"`
	Language    string                    `json:"language,omitempty"`
	LineMatches []sourcegraphRawLineMatch `json:"lineMatches,omitempty"`
}

type sourcegraphRawLineMatch struct {
	Line       string `json:"line"`
	LineNumber int    `json:"lineNumber"`
}

func (m sourcegraphRawMatch) view() sourcegraphMatch {
	lines := make([]sourcegraphLineMatch, len(m.LineMatches))
	for i, line := range m.LineMatches {
		lines[i] = sourcegraphLineMatch(line)
	}
	return sourcegraphMatch{
		Type:        m.Type,
		Repository:  m.Repository,
		Path:        m.Path,
		Commit:      m.Commit,
		Language:    m.Language,
		LineMatches: lines,
	}
}
