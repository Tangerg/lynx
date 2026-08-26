package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	toolcontract "github.com/Tangerg/lynx/core/tool"
)

// ToolSource binds an initialized MCP client session to a logical name used to
// deconflict tools across multiple servers.
type ToolSource struct {
	// Name identifies the upstream server in tool prefixes and error
	// messages. Empty is allowed but discouraged when more than one
	// source is in play.
	Name string

	// Session is a live, initialized client session. The wrapper does not own
	// the session; callers are responsible for closing it.
	Session *sdkmcp.ClientSession
}

// ConcurrencyFunc decides whether one remote tool call may overlap other calls
// from the same model response. A false result keeps the call exclusive; a true
// result with an empty key declares no known conflict, while equal non-empty
// keys serialize.
//
// The callback receives the source and remote tool names, an isolated copy of
// the remote annotations, and the raw call arguments. It must be deterministic,
// side-effect-free, and safe for concurrent use because a durable resume may
// plan queued calls again and callers may inspect the capability from multiple
// goroutines.
type ConcurrencyFunc func(
	sourceName, toolName string,
	annotations sdkmcp.ToolAnnotations,
	arguments string,
) (key string, concurrent bool)

// ToolsConfig configures [Tools].
type ToolsConfig struct {
	// Naming maps each remote tool identity to its public name. Nil
	// uses the package default, "<sourceName>_<toolName>" sanitized to the
	// function-name charset accepted by model providers. The function must be
	// deterministic.
	Naming func(sourceName, toolName string) string

	// MetaFunc is applied to every tool produced. Nil forwards no metadata on
	// tool calls.
	MetaFunc MetaFunc

	// Concurrency opts remote tools into a caller-owned scheduling policy. Nil
	// keeps every MCP call exclusive because protocol descriptors do not provide
	// a trustworthy resource-conflict contract. The lynx Agent ToolLoop still
	// commits results in the model's original call order when this policy enables
	// concurrent execution. [AnnotatedReadOnlyConcurrency] is the conservative
	// ready-made policy for trusted descriptors that declare readOnlyHint=true.
	Concurrency ConcurrencyFunc
}

// publicName returns the configured public name or a provider-safe default.
// MCP itself permits names that model providers reject, while calls still need
// to route by the unchanged raw MCP name.
func (c ToolsConfig) publicName(sourceName, toolName string) string {
	if c.Naming != nil {
		return c.Naming(sourceName, toolName)
	}
	if sourceName == "" {
		return sanitizeToolName(toolName)
	}
	return sanitizeToolName(sourceName + "_" + toolName)
}

func sanitizeToolName(name string) string {
	b := make([]byte, 0, len(name))
	for i := range len(name) {
		character := name[i]
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '_', character == '-':
			b = append(b, character)
		default:
			b = append(b, '_')
		}
	}
	return string(b[:min(len(b), 64)])
}

// Tools lists remote MCP tools from sources and wraps them as lynx tools.
func Tools(ctx context.Context, sources []ToolSource, config ToolsConfig) ([]toolcontract.Tool, error) {
	var all []toolcontract.Tool
	seen := make(map[string]struct{})
	for i, src := range sources {
		if src.Session == nil {
			return nil, fmt.Errorf("mcp: tool source %d %q: %w", i, src.Name, ErrNilSession)
		}
		for descriptor, err := range src.Session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp: list tools from source %q: %w", src.Name, err)
			}
			snapshot, err := newDescriptorSnapshot(descriptor)
			if err != nil {
				return nil, fmt.Errorf("mcp: snapshot tool from source %q: %w", src.Name, err)
			}

			name := config.publicName(src.Name, snapshot.name())
			if name == "" {
				return nil, fmt.Errorf("mcp: source %q tool %q has an empty public name", src.Name, snapshot.name())
			}

			remote, err := newRemoteTool(remoteToolConfig{
				source:      src,
				descriptor:  snapshot,
				publicName:  name,
				metaFunc:    config.MetaFunc,
				concurrency: config.Concurrency,
			})
			if err != nil {
				return nil, fmt.Errorf("mcp: wrap tool %q from source %q: %w", snapshot.name(), src.Name, err)
			}

			if _, dup := seen[name]; dup {
				return nil, fmt.Errorf("mcp: duplicate tool name %q after public naming", name)
			}
			seen[name] = struct{}{}
			all = append(all, remote)
		}
	}
	return all, nil
}
