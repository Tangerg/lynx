package toolset

import (
	"fmt"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/tools/httpreq"
	"github.com/Tangerg/scope/tools/web"
	"github.com/Tangerg/scope/tools/web/jina"
	"github.com/Tangerg/scope/tools/web/tavily"
)

// OnlineConfig groups the credentials network-reaching tools need (web /
// httpreq). Empty fields disable the corresponding tool — no tool
// is registered without explicit opt-in, so an offline-only install makes no
// surprise outbound calls.
type OnlineConfig struct {
	// JinaAPIKey enables page fetching through Jina Reader.
	JinaAPIKey string

	// TavilyAPIKey enables web search through Tavily.
	TavilyAPIKey string

	// HTTPAllowedHosts enables the httpreq tool. Pass an explicit allowlist
	// (e.g. ["api.github.com", "*.openai.com"]) — empty keeps the tool disabled
	// so the LLM can't reach arbitrary internal endpoints.
	HTTPAllowedHosts []string
}

// buildOnline instantiates each network-reaching tool whose
// credentials are present in online. These are working-directory
// independent, so they are built once and shared across all resolutions.
// Missing credentials silently skip the corresponding tool — explicit
// opt-in is the safety model. Returns an error only when a configured
// provider fails to build (e.g. invalid HTTP allowlist).
func buildOnline(online OnlineConfig) ([]toolcontract.Tool, error) {
	var (
		out []toolcontract.Tool
		err error
	)

	out, err = appendEnabled(out, online.JinaAPIKey != "", "web fetch (jina)", func() (toolcontract.Tool, error) {
		client, clientErr := jina.NewClient(jina.Config{APIKey: online.JinaAPIKey})
		if clientErr != nil {
			return nil, clientErr
		}
		return web.NewFetchTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendEnabled(out, online.TavilyAPIKey != "", "web search (tavily)", func() (toolcontract.Tool, error) {
		client, clientErr := tavily.NewClient(tavily.Config{APIKey: online.TavilyAPIKey})
		if clientErr != nil {
			return nil, clientErr
		}
		return web.NewSearchTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendEnabled(out, len(online.HTTPAllowedHosts) > 0, "httpreq", func() (toolcontract.Tool, error) {
		client, clientErr := httpreq.NewClient(httpreq.ClientConfig{AllowedHosts: online.HTTPAllowedHosts})
		if clientErr != nil {
			return nil, clientErr
		}
		return httpreq.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// appendEnabled conditionally registers one configured network capability. When
// cond is false it returns tools unchanged (the credentials weren't
// supplied so the tool stays disabled — explicit opt-in is the
// safety model). When cond is true it runs build(); a non-nil
// error is wrapped with the label so the caller can tell which
// provider mis-configured.
func appendEnabled(tools []toolcontract.Tool, cond bool, label string, build func() (toolcontract.Tool, error)) ([]toolcontract.Tool, error) {
	if !cond {
		return tools, nil
	}
	tool, err := build()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return append(tools, tool), nil
}
