package toolset

import (
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/tools/httpreq"
	"github.com/Tangerg/lynx/tools/webfetch"
	"github.com/Tangerg/lynx/tools/webfetch/jina"
	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/tavily"
)

// OnlineConfig groups the credentials network-reaching tools need (webfetch /
// websearch / httpreq). Empty fields disable the corresponding tool — no tool
// is registered without explicit opt-in, so an offline-only install makes no
// surprise outbound calls.
type OnlineConfig struct {
	// JinaAPIKey enables the webfetch tool backed by Jina Reader.
	JinaAPIKey string

	// TavilyAPIKey enables the websearch tool backed by Tavily.
	TavilyAPIKey string

	// HTTPAllowedHosts enables the httpreq tool. Pass an explicit allowlist
	// (e.g. ["api.github.com", "*.openai.com"]) — empty keeps the tool disabled
	// so the LLM can't reach arbitrary internal endpoints.
	HTTPAllowedHosts []string

	// SourcegraphEndpoint enables sourcegraph_search. For Sourcegraph Cloud use
	// "https://sourcegraph.com"; private instances may pass their base URL.
	SourcegraphEndpoint string

	// SourcegraphToken is optional for Sourcegraph Cloud public-code searches
	// and required for most private instances.
	SourcegraphToken string
}

// BuildOnlineTools instantiates each network-reaching tool whose
// credentials are present in online. These are working-directory
// independent, so they are built once and shared across all resolutions.
// Missing credentials silently skip the corresponding tool — explicit
// opt-in is the safety model. Returns an error only when a configured
// provider fails to build (e.g. invalid HTTP allowlist).
func BuildOnlineTools(online OnlineConfig) ([]chat.Tool, error) {
	var (
		out []chat.Tool
		err error
	)

	out, err = appendIfBuilt(out, online.JinaAPIKey != "", "webfetch (jina)", func() (chat.Tool, error) {
		client, clientErr := jina.NewClient(&jina.Config{APIKey: online.JinaAPIKey})
		if clientErr != nil {
			return nil, clientErr
		}
		return webfetch.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, online.TavilyAPIKey != "", "websearch (tavily)", func() (chat.Tool, error) {
		client, clientErr := tavily.NewClient(&tavily.Config{APIKey: online.TavilyAPIKey})
		if clientErr != nil {
			return nil, clientErr
		}
		return websearch.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, len(online.HTTPAllowedHosts) > 0, "httpreq", func() (chat.Tool, error) {
		client, clientErr := httpreq.NewClient(httpreq.Config{AllowedHosts: online.HTTPAllowedHosts})
		if clientErr != nil {
			return nil, clientErr
		}
		return httpreq.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, online.SourcegraphEndpoint != "", "sourcegraph", func() (chat.Tool, error) {
		return newSourcegraphTool(sourcegraphConfig{
			Endpoint: online.SourcegraphEndpoint,
			Token:    online.SourcegraphToken,
		})
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// appendIfBuilt is the conditional-tool-registration helper. When
// cond is false it returns tools unchanged (the credentials weren't
// supplied so the tool stays disabled — explicit opt-in is the
// safety model). When cond is true it runs build(); a non-nil
// error is wrapped with the label so the caller can tell which
// provider mis-configured.
func appendIfBuilt(tools []chat.Tool, cond bool, label string, build func() (chat.Tool, error)) ([]chat.Tool, error) {
	if !cond {
		return tools, nil
	}
	tool, err := build()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return append(tools, tool), nil
}
