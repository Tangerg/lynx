package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	toolcontract "github.com/Tangerg/lynx/tools"
)

// defaultToolNaming returns "<sourceName>_<toolName>" (or the bare tool name
// when sourceName is empty), sanitized to the tool-name charset LLM providers
// accept. Used when Options.Naming is left nil.
//
// MCP places no charset constraint on tool / server names, but the model
// providers do — Anthropic and OpenAI both require function names to match
// ^[a-zA-Z0-9_-]{1,64}$. An un-sanitized name (e.g. a server named
// "html.to.design" → "html.to.design_import-url", with dots) makes the WHOLE
// chat request invalid, so the provider rejects every turn. Mapping the public
// label onto the provider charset is the client's job; the call still routes by
// the raw MCP tool name ([tool.Call] uses the descriptor, not this label), so
// the remote tool is unaffected.
func defaultToolNaming(sourceName string, tool *sdkmcp.Tool) string {
	if sourceName == "" {
		return sanitizeToolName(tool.Name)
	}
	return sanitizeToolName(sourceName + "_" + tool.Name)
}

// sanitizeToolName maps name onto the provider-accepted tool-name charset
// (^[a-zA-Z0-9_-]{1,64}$): every other byte becomes '_', and the result is
// capped at 64. Deterministic (ToolOptions.Naming's contract); the output is pure
// ASCII so the length cap can't split a rune.
func sanitizeToolName(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) > 64 {
		b = b[:64]
	}
	return string(b)
}

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
// The callback receives the source's logical name, the remote descriptor, and
// the raw call arguments. It must be deterministic, side-effect-free, and safe
// for concurrent use because a durable resume may plan queued calls again and
// callers may inspect the capability from multiple goroutines. Treat tool as
// read-only.
type ConcurrencyFunc func(sourceName string, tool *sdkmcp.Tool, arguments string) (key string, concurrent bool)

// ToolOptions configures [Tools].
type ToolOptions struct {
	// Naming maps each remote tool descriptor to its public name. Nil
	// uses the package default, "<sourceName>_<toolName>" sanitized to the
	// function-name charset accepted by model providers. The function must be
	// deterministic.
	Naming func(sourceName string, tool *sdkmcp.Tool) string

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

func (o ToolOptions) withDefaults() ToolOptions {
	if o.Naming == nil {
		o.Naming = defaultToolNaming
	}
	return o
}

// Tools lists remote MCP tools from sources and wraps them as chat tools.
func Tools(ctx context.Context, sources []ToolSource, opts ToolOptions) ([]toolcontract.Tool, error) {
	opts = opts.withDefaults()
	var all []toolcontract.Tool
	seen := make(map[string]struct{})
	for i, src := range sources {
		if src.Session == nil {
			return nil, fmt.Errorf("mcp.Tools: source[%d] %q: %w", i, src.Name, ErrNilSession)
		}
		for descriptor, err := range src.Session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp.Tools: list tools from source %q: %w", src.Name, err)
			}
			if descriptor == nil {
				return nil, fmt.Errorf("mcp.Tools: source %q: %w", src.Name, errNilDescriptor)
			}

			name := opts.Naming(src.Name, descriptor)
			if name == "" {
				return nil, fmt.Errorf("mcp.Tools: source %q tool %q: public name must not be empty", src.Name, descriptor.Name)
			}

			tool, err := newTool(toolConfig{
				Session:      src.Session,
				Descriptor:   descriptor,
				PrefixedName: name,
				MetaFunc:     opts.MetaFunc,
				SourceName:   src.Name,
				Concurrency:  opts.Concurrency,
			})
			if err != nil {
				return nil, fmt.Errorf("mcp.Tools: wrap tool %q from source %q: %w", descriptor.Name, src.Name, err)
			}

			if _, dup := seen[name]; dup {
				return nil, fmt.Errorf("mcp.Tools: duplicate tool name after prefixing: %q", name)
			}
			seen[name] = struct{}{}
			all = append(all, tool)
		}
	}
	return all, nil
}
