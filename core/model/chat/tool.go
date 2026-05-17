package chat

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ToolDefinition is the static description of a tool that LLMs see when
// deciding whether and how to call it. The InputSchema is a JSON Schema
// the model uses to format its arguments.
type ToolDefinition struct {
	// Name uniquely identifies the tool. Required.
	Name string

	// Description is a human-readable hint shown to the LLM.
	Description string

	// InputSchema is a JSON Schema describing the argument shape.
	// Required so the LLM can format arguments correctly.
	InputSchema string
}

// ToolMetadata controls how the framework treats a tool's result after
// execution.
type ToolMetadata struct {
	// ReturnDirect routes the tool result straight back to the caller
	// without re-prompting the LLM. Useful for UI affordances and
	// notifications. False (the default) sends the result back to the
	// LLM for integration into the next reply.
	ReturnDirect bool
}

// Tool is the static surface every tool exposes. Implementations that
// also satisfy [CallableTool] run inline; bare-Tool implementations
// are delegated back to the caller — see [NewTool].
type Tool interface {
	// Definition returns the static description shown to the LLM.
	Definition() ToolDefinition

	// Metadata returns the post-execution behavior (return-direct, ...).
	Metadata() ToolMetadata
}

// CallableTool is the inline-execution flavor of [Tool]. The framework
// invokes [CallableTool.Call] when the LLM requests this tool by name.
type CallableTool interface {
	Tool

	// Call runs the tool's body. arguments is the JSON-encoded payload
	// the LLM produced. The string result is fed back to the LLM (or
	// returned to the caller when ReturnDirect is true).
	Call(ctx context.Context, arguments string) (string, error)
}

// baseTool is the shared backing for both delegated and inline tools.
type baseTool struct {
	definition ToolDefinition
	metadata   ToolMetadata
}

func (t *baseTool) Definition() ToolDefinition { return t.definition }
func (t *baseTool) Metadata() ToolMetadata     { return t.metadata }

// callableTool wraps baseTool with an exec function for inline tools.
type callableTool struct {
	baseTool
	execFunc func(ctx context.Context, arguments string) (string, error)
}

// Call runs the tool's exec function; returns an error when the tool was
// constructed without one.
func (t *callableTool) Call(ctx context.Context, arguments string) (string, error) {
	if t.execFunc == nil {
		return "", fmt.Errorf("chat.callableTool.Call: tool %q has no exec function", t.definition.Name)
	}
	return t.execFunc(ctx, arguments)
}

// NewTool builds a [Tool]. If execFunc is nil the result is a delegated
// (external) tool that satisfies [Tool] only; otherwise it is an inline
// (internal) tool that also satisfies [CallableTool].
//
// Pass nil for execFunc to build a delegated tool — the framework
// will surface the call request to the host instead of running it.
//
// Example:
//
//	tool, err := chat.NewTool(
//	    chat.ToolDefinition{Name: "add", InputSchema: addSchema},
//	    chat.ToolMetadata{},
//	    func(ctx context.Context, args string) (string, error) { ... },
//	)
func NewTool(definition ToolDefinition, metadata ToolMetadata, execFunc func(ctx context.Context, arguments string) (string, error)) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("chat.NewTool: definition.Name must not be empty")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("chat.NewTool: definition.InputSchema must not be empty")
	}

	base := baseTool{definition: definition, metadata: metadata}
	if execFunc == nil {
		return &base, nil
	}
	return &callableTool{baseTool: base, execFunc: execFunc}, nil
}

// ToolRegistry is a thread-safe map from tool name to [Tool] instance.
// Registration is idempotent (duplicate names are silently ignored), so
// concurrent boot-time setup is safe.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// newToolRegistry builds an empty registry. capacityHint, if positive,
// preallocates the backing map.
func newToolRegistry(capacityHint ...int) *ToolRegistry {
	capacity, _ := pkgSlices.First(capacityHint)
	if capacity < 0 {
		capacity = 0
	}
	return &ToolRegistry{tools: make(map[string]Tool, capacity)}
}

// Register adds tools using their definition Name as the key. Duplicate
// names are silently dropped — first writer wins. Returns the registry
// for chaining.
func (r *ToolRegistry) Register(tools ...Tool) *ToolRegistry {
	if len(tools) == 0 {
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := tool.Definition().Name
		if _, exists := r.tools[name]; !exists {
			r.tools[name] = tool
		}
	}
	return r
}

// Unregister removes tools by name. Unknown names are silently ignored.
// Returns the registry for chaining.
func (r *ToolRegistry) Unregister(names ...string) *ToolRegistry {
	if len(names) == 0 {
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		delete(r.tools, name)
	}
	return r
}

// Find looks up a tool by name.
func (r *ToolRegistry) Find(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, exists := r.tools[name]
	return tool, exists
}

// Exists reports whether a tool with the given name is registered.
func (r *ToolRegistry) Exists(name string) bool {
	_, ok := r.Find(name)
	return ok
}

// All returns a snapshot of every registered tool. Mutations to the
// returned slice do not affect the registry.
func (r *ToolRegistry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		out = append(out, tool)
	}
	return out
}

// Names returns a snapshot of every registered tool name.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

// Size returns the number of registered tools.
func (r *ToolRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Clear removes every registered tool. Returns the registry for chaining.
func (r *ToolRegistry) Clear() *ToolRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.tools)
	return r
}

// ToolInvocationResult is what the tool-calling middleware emits after
// running the LLM-requested tool calls. It captures both the inline
// results (toolMessage) and any external requests (externalToolCalls)
// that the host needs to execute, plus the flow-control bit
// (allReturnDirect) that decides whether to feed results back to the
// LLM or return them to the caller.
type ToolInvocationResult struct {
	request           *Request
	response          *Response
	toolMessage       *ToolMessage
	allReturnDirect   bool
	externalToolCalls []*ToolCall
}

// ShouldContinue reports whether the runtime should re-prompt the LLM
// with the tool results. It is true only when:
//   - no external tools are pending (those always force direct return), AND
//   - at least one internal tool wants its result fed back to the LLM.
func (r *ToolInvocationResult) ShouldContinue() bool {
	if len(r.externalToolCalls) > 0 {
		return false
	}
	return !r.allReturnDirect
}

// ShouldReturn is the inverse of [ToolInvocationResult.ShouldContinue].
func (r *ToolInvocationResult) ShouldReturn() bool { return !r.ShouldContinue() }

// BuildContinueRequest assembles the next [*Request] in the tool-calling
// loop: the original conversation plus the assistant's tool-call message
// plus the [*ToolMessage] carrying inline results. Returns an error when
// the result is not actually in "continue" state.
func (r *ToolInvocationResult) BuildContinueRequest() (*Request, error) {
	if !r.ShouldContinue() {
		return nil, errors.New("chat.ToolInvocationResult.BuildContinueRequest: result is in return-direct state")
	}
	if err := r.assertContinuableState(); err != nil {
		return nil, err
	}

	result := r.response.Result
	if result == nil || !result.AssistantMessage.HasToolCalls() {
		return nil, errors.New("chat.ToolInvocationResult.BuildContinueRequest: response has no tool calls")
	}

	msgs := append(r.request.Messages, result.AssistantMessage, r.toolMessage)
	next, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = r.request.Options.Clone()
	next.Tools = slices.Clone(r.request.Tools)
	next.Params = maps.Clone(r.request.Params)
	return next, nil
}

// assertContinuableState validates that the inputs needed to build the
// continuation request are present.
func (r *ToolInvocationResult) assertContinuableState() error {
	if r.request == nil {
		return errors.New("chat.ToolInvocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("chat.ToolInvocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("chat.ToolInvocationResult: internal-tools message is missing")
	}
	return nil
}

// BuildReturnResponse assembles the final [*Response] when no further
// LLM round is needed — either every internal tool was return-direct, or
// external tool calls are pending and the host must execute them.
func (r *ToolInvocationResult) BuildReturnResponse() (*Response, error) {
	if !r.ShouldReturn() {
		return nil, errors.New("chat.ToolInvocationResult.BuildReturnResponse: result is in continue state")
	}
	if r.response == nil {
		return nil, errors.New("chat.ToolInvocationResult.BuildReturnResponse: LLM response is missing")
	}

	withCalls := r.response.Result
	if withCalls == nil || !withCalls.AssistantMessage.HasToolCalls() {
		return nil, errors.New("chat.ToolInvocationResult.BuildReturnResponse: response has no tool calls")
	}
	original := withCalls.AssistantMessage

	modified := NewAssistantMessage(MessageParams{
		Text:      original.Text,
		Media:     original.Media,
		ToolCalls: r.externalToolCalls,
		Metadata:  original.Metadata,
	})

	result, err := NewResult(modified, withCalls.Metadata)
	if err != nil {
		return nil, fmt.Errorf("chat.ToolInvocationResult.BuildReturnResponse: %w", err)
	}
	result.ToolMessage = r.toolMessage

	return NewResponse(result, r.response.Metadata)
}

// validate ensures the result has at least one result channel
// populated — either the inline tool message or the external call list.
func (r *ToolInvocationResult) validate() error {
	if r.request == nil {
		return errors.New("chat.ToolInvocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("chat.ToolInvocationResult: LLM response is missing")
	}
	if r.toolMessage == nil && len(r.externalToolCalls) == 0 {
		return errors.New("chat.ToolInvocationResult: at least one of internal-tools message or external tool calls is required")
	}
	return nil
}

// toolCallInvoker drives one round of tool invocations: identify which
// tools need to run, execute the inline ones, collect the external ones,
// and assemble the [*ToolInvocationResult].
type toolCallInvoker struct {
	registry *ToolRegistry
}

// newToolCallInvoker pairs an invoker with its registry.
func newToolCallInvoker(registry *ToolRegistry) *toolCallInvoker {
	return &toolCallInvoker{registry: registry}
}

// canInvokeToolCalls verifies every requested tool name is registered.
// Returns (false, nil) when the response contains no tool calls at all.
// Returns (false, err) when an unknown tool is requested.
func (i *toolCallInvoker) canInvokeToolCalls(resp *Response) (bool, error) {
	if resp.Result == nil || !resp.Result.AssistantMessage.HasToolCalls() {
		return false, nil
	}

	for _, call := range resp.Result.AssistantMessage.ToolCalls {
		if _, exists := i.registry.Find(call.Name); !exists {
			return false, fmt.Errorf("chat.toolCallInvoker.canInvokeToolCalls: tool %q not registered", call.Name)
		}
	}
	return true, nil
}

// invokeToolCalls runs the inline tools in order and collects the
// external ones into a separate slice for the host to handle.
func (i *toolCallInvoker) invokeToolCalls(ctx context.Context, calls []*ToolCall) (*ToolInvocationResult, error) {
	var (
		external        []*ToolCall
		allReturnDirect = true
		internal        []*ToolReturn
	)

	for _, call := range calls {
		// Existence is guaranteed by the canInvokeToolCalls precheck.
		tool, _ := i.registry.Find(call.Name)

		callable, ok := tool.(CallableTool)
		if !ok {
			external = append(external, call)
			continue
		}

		result, err := callable.Call(ctx, call.Arguments)
		if err != nil {
			return nil, fmt.Errorf("chat.toolCallInvoker.invokeToolCalls: tool %q failed: %w", call.Name, err)
		}

		allReturnDirect = allReturnDirect && callable.Metadata().ReturnDirect
		internal = append(internal, &ToolReturn{
			ID:     call.ID,
			Name:   call.Name,
			Result: result,
		})
	}

	var (
		toolMsg *ToolMessage
		err     error
	)
	if len(internal) > 0 {
		toolMsg, err = NewToolMessage(internal)
		if err != nil {
			return nil, fmt.Errorf("chat.toolCallInvoker.invokeToolCalls: %w", err)
		}
	}

	return &ToolInvocationResult{
		toolMessage:       toolMsg,
		externalToolCalls: external,
		allReturnDirect:   allReturnDirect,
	}, nil
}

// invoke is the orchestrator: validate, run, attach context.
func (i *toolCallInvoker) invoke(ctx context.Context, req *Request, resp *Response) (*ToolInvocationResult, error) {
	canInvoke, err := i.canInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !canInvoke {
		return nil, errors.New("chat.toolCallInvoker.invoke: response has no valid tool calls")
	}

	result, err := i.invokeToolCalls(ctx, resp.Result.AssistantMessage.ToolCalls)
	if err != nil {
		return nil, err
	}
	result.request = req
	result.response = resp

	return result, result.validate()
}

// ToolSupport bundles a [ToolRegistry] with a tool-call invoker — the
// integration point the [NewToolMiddleware] middleware uses to drive
// the tool-calling loop. See [NewToolMiddleware] for end-to-end wiring.
type ToolSupport struct {
	registry *ToolRegistry
	invoker  *toolCallInvoker
}

// NewToolSupport returns a [ToolSupport] backed by a fresh registry.
// capacityHint, if positive, preallocates the registry's backing map.
func NewToolSupport(capacityHint ...int) *ToolSupport {
	registry := newToolRegistry(capacityHint...)
	return &ToolSupport{
		registry: registry,
		invoker:  newToolCallInvoker(registry),
	}
}

// Registry exposes the underlying [ToolRegistry] for direct access.
func (s *ToolSupport) Registry() *ToolRegistry { return s.registry }

// Register is a shorthand for [ToolRegistry.Register].
func (s *ToolSupport) Register(tools ...Tool) {
	s.registry.Register(tools...)
}

// Unregister is a shorthand for [ToolRegistry.Unregister].
func (s *ToolSupport) Unregister(names ...string) {
	s.registry.Unregister(names...)
}

// ShouldReturnDirect reports whether the conversation should end with
// the most recent tool message (no further LLM round). It is true only
// when:
//   - the last message is a [*ToolMessage], AND
//   - every tool referenced in that message is registered, AND
//   - every such tool has ReturnDirect = true.
func (s *ToolSupport) ShouldReturnDirect(msgs []Message) bool {
	if !hasMessageTypeAtLast(msgs, MessageTypeTool) {
		return false
	}

	last, _ := pkgSlices.Last(msgs)
	toolMsg, ok := last.(*ToolMessage)
	if !ok {
		return false
	}

	for _, ret := range toolMsg.ToolReturns {
		tool, exists := s.registry.Find(ret.Name)
		if !exists {
			return false
		}
		if !tool.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// BuildReturnDirectResponse assembles a synthetic [*Response] that wraps
// the last [*ToolMessage] as the final answer. Returns an error when
// [ToolSupport.ShouldReturnDirect] would return false.
func (s *ToolSupport) BuildReturnDirectResponse(msgs []Message) (*Response, error) {
	if !s.ShouldReturnDirect(msgs) {
		return nil, errors.New("chat.ToolSupport.BuildReturnDirectResponse: conditions for return-direct are not met")
	}
	last, _ := pkgSlices.Last(msgs)

	assistantMsg := NewAssistantMessage(map[string]any{
		"created_by": FinishReasonReturnDirect.String(),
	})
	metadata := &ResultMetadata{FinishReason: FinishReasonReturnDirect}

	result, err := NewResult(assistantMsg, metadata)
	if err != nil {
		return nil, fmt.Errorf("chat.ToolSupport.BuildReturnDirectResponse: %w", err)
	}

	// ShouldReturnDirect already verified this is a *ToolMessage.
	result.ToolMessage = last.(*ToolMessage)

	return NewResponse(result, &ResponseMetadata{})
}

// ShouldInvokeToolCalls reports whether the response contains tool
// calls that the registry can fulfil.
func (s *ToolSupport) ShouldInvokeToolCalls(resp *Response) (bool, error) {
	return s.invoker.canInvokeToolCalls(resp)
}

// InvokeToolCalls runs one tool-calling round: validate, dispatch to
// inline tools, collect external ones, and assemble the result.
//
// Flow control:
//   - any external tool forces a direct return,
//   - among internal tools, any non-return-direct keeps the loop going,
//   - only when every internal tool is return-direct does the loop end.
func (s *ToolSupport) InvokeToolCalls(ctx context.Context, req *Request, resp *Response) (*ToolInvocationResult, error) {
	return s.invoker.invoke(ctx, req, resp)
}
