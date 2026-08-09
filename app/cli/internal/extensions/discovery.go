package extensions

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Source discovers plugins without deciding their load order or lifecycle.
// A source can represent compiled built-ins, an embedding host, or another
// trust boundary with its own validation and loading mechanism.
type Source interface {
	ID() string
	Discover(context.Context) (SourceResult, error)
}

// SourceResult permits a source to isolate malformed plugin entries without
// discarding its valid discoveries.
type SourceResult struct {
	Plugins []Plugin
	Issues  []error
}

// StaticSource is the concrete source for compiled and host-injected plugins.
type StaticSource struct {
	Name    string
	Plugins []Plugin
}

func (s StaticSource) ID() string {
	if name := strings.TrimSpace(s.Name); name != "" {
		return name
	}
	return "static"
}

func (s StaticSource) Discover(context.Context) (SourceResult, error) {
	out := make([]Plugin, len(s.Plugins))
	for i, plugin := range s.Plugins {
		out[i] = clonePlugin(plugin)
	}
	return SourceResult{Plugins: out}, nil
}

// SourceIssue isolates one failed source while allowing independent sources to
// remain available.
type SourceIssue struct {
	Source string
	Err    error
}

// Discovery is one immutable collection pass.
type Discovery struct {
	Plugins []Plugin
	Issues  []SourceIssue
}

// Discover collects sources in declaration order. Cancellation stops the pass;
// a source-local failure is recorded and does not erase prior discoveries.
func Discover(ctx context.Context, sources ...Source) (Discovery, error) {
	var result Discovery
	for i, source := range sources {
		if err := context.Cause(ctx); err != nil {
			return Discovery{}, err
		}
		if source == nil {
			result.Issues = append(result.Issues, SourceIssue{Source: fmt.Sprintf("source-%d", i+1), Err: fmt.Errorf("extensions: plugin source %d is nil", i+1)})
			continue
		}
		discovered, err := source.Discover(ctx)
		if err != nil {
			result.Issues = append(result.Issues, SourceIssue{Source: source.ID(), Err: fmt.Errorf("discover plugins from %q: %w", source.ID(), err)})
			continue
		}
		for _, issue := range discovered.Issues {
			if issue != nil {
				result.Issues = append(result.Issues, SourceIssue{Source: source.ID(), Err: issue})
			}
		}
		for _, plugin := range discovered.Plugins {
			result.Plugins = append(result.Plugins, clonePlugin(plugin))
		}
	}
	result.Plugins = slices.Clip(result.Plugins)
	result.Issues = slices.Clip(result.Issues)
	return result, nil
}
