package toolloop

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// hostedTool is one host-supplied tool the loop interrogates across the trust
// boundary. Every question below runs code the framework does not own, so each
// answer arrives as an error rather than a panic, attributed to the name the
// model called.
//
// Name and tool travel together because neither alone can answer a question.
// The loop knows the advertised name; the tool value cannot supply it, since a
// decorator chain may report a different Definition than the one advertised —
// which is exactly the mismatch [executableFor] exists to catch. Resolving a
// name is therefore where the pair is formed, and nothing downstream has to
// carry the two separately.
type hostedTool struct {
	tool tools.Tool
	name string
}

// resolveHosted looks name up in resolver and pairs the result with it. A false
// ok means the resolver has no such tool, which is the caller's decision to
// interpret: advertising an unresolvable tool is an error, while a promotion
// that no longer resolves is simply dropped.
func resolveHosted(resolver ToolResolver, name string) (hosted hostedTool, ok bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			hosted, ok = hostedTool{}, false
			err = panicerr.New(fmt.Sprintf("tool resolver %T Resolve(%q) panicked", resolver, name), recovered)
		}
	}()
	tool, ok := resolver.Resolve(name)
	if !ok || valueIsNil(tool) {
		return hostedTool{}, false, nil
	}
	return hostedTool{tool: tool, name: name}, true, nil
}

// executableFor resolves definition's name and confirms the executable tool
// describes itself the same way. A model told about one schema must not reach a
// tool that expects another, and a decorator that rewrote Definition on the way
// out is how that happens. A false matched is the honest "resolved, but not the
// tool that was advertised".
func executableFor(resolver ToolResolver, definition chat.ToolDefinition) (hostedTool, bool, error) {
	hosted, ok, err := resolveHosted(resolver, definition.Name)
	if err != nil || !ok {
		return hostedTool{}, false, err
	}
	executable, err := hosted.definition()
	if err != nil {
		return hostedTool{}, false, err
	}
	return hosted, sameToolDefinition(definition, executable), nil
}

// label is how an error refers to this tool: the advertised name when the loop
// knows it, and the Go type when it does not — Advertise interrogates candidates
// before any name exists, because the name is what Definition returns.
func (t hostedTool) label() string {
	if t.name != "" {
		return strconv.Quote(t.name)
	}
	return fmt.Sprintf("%T", t.tool)
}

// definition asks the tool how it describes itself.
func (t hostedTool) definition() (definition chat.ToolDefinition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool %s Definition panicked", t.label()), recovered)
		}
	}()
	return t.tool.Definition(), nil
}

// concurrencyKey owns both halves of asking a tool how it schedules: finding
// the declaration through the wrapping chain and calling it. An undeclared tool
// is exclusive, which is why a missing capability is not an error.
func (t hostedTool) concurrencyKey(arguments string) (key string, concurrent bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			key, concurrent = "", false
			err = panicerr.New(fmt.Sprintf("tool %s concurrency lookup panicked", t.label()), recovered)
		}
	}()
	declared, ok, err := tools.Capability[ConcurrentTool](t.tool)
	if err != nil {
		return "", false, fmt.Errorf("tool %s concurrency lookup: %w", t.label(), err)
	}
	if !ok {
		return "", false, nil
	}
	key, concurrent = declared.ConcurrencyKey(arguments)
	return key, concurrent, nil
}

// returnsDirect reports whether the tool ends the loop with its own output.
func (t hostedTool) returnsDirect() (direct bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			direct = false
			err = panicerr.New(fmt.Sprintf("tool %s direct-return lookup panicked", t.label()), recovered)
		}
	}()
	marker, ok, err := tools.Capability[DirectTool](t.tool)
	if err != nil {
		return false, fmt.Errorf("tool %s direct-return lookup: %w", t.label(), err)
	}
	if !ok {
		return false, nil
	}
	return marker.ReturnsDirect(), nil
}

func (t hostedTool) canContinueWithoutInput() (allowed bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			allowed = false
			err = panicerr.New(fmt.Sprintf("tool %s inputless-continuation lookup panicked", t.label()), recovered)
		}
	}()
	marker, ok, err := tools.Capability[InputlessContinuationTool](t.tool)
	if err != nil {
		return false, fmt.Errorf("tool %s inputless-continuation lookup: %w", t.label(), err)
	}
	if !ok {
		return false, nil
	}
	return marker.CanContinueWithoutInput(), nil
}

// deferredNames asks a deferring tool which tools it can promote.
func (t hostedTool) deferredNames(deferred DeferredTool) (names []string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			names = nil
			err = panicerr.New(fmt.Sprintf("tool %s deferred-name lookup panicked", t.label()), recovered)
		}
	}()
	return deferred.DeferredToolNames(), nil
}

// call runs the tool. The message stays unattributed because it reaches the
// model as a tool result, where the call it answers already names the tool.
func (t hostedTool) call(ctx context.Context, arguments string) (output string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			output = ""
			err = panicerr.New("tool panicked", recovered)
		}
	}()
	return t.tool.Call(ctx, arguments)
}
