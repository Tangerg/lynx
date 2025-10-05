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

// ToolDefinition represents a tool definition that enables LLM models to understand
// when and how to invoke external functions.
//
// Contains essential metadata for LLM tool calling:
//   - Name: Unique tool identifier for LLM recognition
//   - Description: Human-readable explanation for LLM decision-making
//   - InputSchema: JSON Schema defining required input parameter structure
type ToolDefinition struct {
	Name        string // unique identifier for tool recognition
	Description string // descriptive text guiding LLM usage decisions
	InputSchema string // JSON Schema for parameter validation
}

// ToolMetadata represents execution configuration that controls how the LLM framework
// processes tool results.
type ToolMetadata struct {
	// ReturnDirect determines whether tool results bypass further LLM processing.
	// When true, results are returned directly to the user (e.g., UI interactions, notifications).
	// When false, results are passed back to the LLM for integration and further processing.
	ReturnDirect bool
}

// ToolContext implements the ToolContext interface with thread-safe key-value storage.
type ToolContext struct {
	ctx    context.Context
	mu     sync.RWMutex   // Protects concurrent access to fields
	fields map[string]any // Thread-safe key-value storage
}

// NewToolContext creates a new thread-safe ToolContext instance with the provided context.
func NewToolContext(ctx context.Context) *ToolContext {
	return &ToolContext{
		ctx:    ctx,
		fields: make(map[string]any),
	}
}

func (t *ToolContext) Context() context.Context {
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

func (t *ToolContext) Set(key string, val any) *ToolContext {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.fields[key] = val
	return t
}

func (t *ToolContext) SetMap(m map[string]any) *ToolContext {
	if len(m) == 0 {
		return t
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for k, v := range m {
		t.fields[k] = v
	}
	return t
}

func (t *ToolContext) Get(key string) (any, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	v, ok := t.fields[key]
	return v, ok
}

func (t *ToolContext) Fields() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return maps.Clone(t.fields)
}

func (t *ToolContext) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	clear(t.fields)
}

func (t *ToolContext) Clone() *ToolContext {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &ToolContext{
		ctx:    t.ctx,
		fields: maps.Clone(t.fields),
	}
}

// Tool represents a tool definition that can be invoked by LLM models.
//
// Execution Patterns:
// The framework supports two distinct execution patterns:
//
// 1. External Tools (delegation pattern):
//   - Require client-side execution (e.g., user interactions, file operations)
//   - Implement only the Tool interface
//   - Framework delegates execution to external environment
//   - Results always return directly to user (ReturnDirect setting ignored)
//
// 2. Internal Tools (direct execution pattern):
//   - Have built-in execution capability (e.g., calculations, API calls)
//   - Implement both Tool and CallableTool interfaces
//   - Framework executes directly via Call method
//   - Typically configured with ReturnDirect=false for LLM integration
type Tool interface {
	// Definition returns the tool definition containing metadata
	// that guides LLM decision-making.
	Definition() ToolDefinition

	// Metadata returns the execution configuration that defines
	// behavior settings for tool invocations.
	Metadata() ToolMetadata
}

// CallableTool extends Tool with internal execution capability.
// Tools implementing this interface contain an execution function
// that provides consistent behavior across invocations.
type CallableTool interface {
	Tool

	// Call executes the tool's business logic within the framework.
	//
	// Parameters:
	//   - ctx: Execution context with conversation state and environment information
	//   - arguments: Input parameters, typically in JSON format
	//
	// Returns:
	//   - string: Execution result for LLM processing or direct user output
	//   - error: Execution error if the operation fails
	Call(ctx *ToolContext, arguments string) (string, error)
}

// tool provides the base implementation for external tools requiring delegation.
type tool struct {
	definition ToolDefinition
	metadata   ToolMetadata
}

func (t *tool) Definition() ToolDefinition {
	return t.definition
}

func (t *tool) Metadata() ToolMetadata {
	return t.metadata
}

// callableTool provides the implementation for internal tools with execution capability.
// Combines base properties with an execution function.
type callableTool struct {
	tool
	caller func(ctx *ToolContext, input string) (string, error) // execution function
}

func (t *callableTool) Call(ctx *ToolContext, input string) (string, error) {
	if t.caller == nil {
		return "", fmt.Errorf("execution function is required for tool %s", t.definition.Name)
	}
	return t.caller(ctx, input)
}

// NewTool creates a new tool instance.
// If caller is provided, returns a CallableTool; otherwise returns a Tool for external execution.
//
// Parameters:
//   - definition: Tool metadata and schema information
//   - metadata: Execution behavior configuration
//   - caller: Optional execution function (nil for external tools)
//
// Returns:
//   - Tool: External tool (when caller is nil) or CallableTool (when caller is provided)
//   - error: Validation error if required fields are missing
func NewTool(definition ToolDefinition, metadata ToolMetadata, caller func(ctx *ToolContext, input string) (string, error)) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("tool name cannot be empty")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("tool input schema cannot be empty")
	}

	t := tool{
		definition: definition,
		metadata:   metadata,
	}

	if caller == nil {
		return &t, nil
	}

	return &callableTool{
		tool:   t,
		caller: caller,
	}, nil
}

// ToolRegistry provides thread-safe management of immutable tool instances for LLM applications.
// Uses tool names as unique identifiers and prevents duplicate registrations.
// All operations are concurrent-safe and work with immutable tools that cannot be modified after creation.
type ToolRegistry struct {
	mu    sync.RWMutex    // Protects concurrent access to the store
	store map[string]Tool // Maps tool names to immutable Tool instances
}

// newToolRegistry creates a new registry with optional initial capacity.
// Negative capacity values default to 0.
func newToolRegistry(cap ...int) *ToolRegistry {
	c, _ := pkgSlices.First(cap)
	if c < 0 {
		c = 0
	}
	return &ToolRegistry{
		store: make(map[string]Tool, c),
	}
}

// Register adds immutable tools to the registry using their names as identifiers.
// Duplicate names are silently ignored to prevent overwriting existing tools.
// Returns the registry for method chaining.
func (r *ToolRegistry) Register(tools ...Tool) *ToolRegistry {
	if len(tools) == 0 {
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Definition().Name
		if _, exists := r.store[name]; !exists {
			r.store[name] = t
		}
	}
	return r
}

// Unregister removes tools by name from the registry.
// Non-existent names are silently ignored.
// Returns the registry for method chaining.
func (r *ToolRegistry) Unregister(names ...string) *ToolRegistry {
	if len(names) == 0 {
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, name := range names {
		delete(r.store, name)
	}
	return r
}

// Find retrieves a tool by name.
// Returns the tool and true if found, nil and false otherwise.
func (r *ToolRegistry) Find(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.store[name]
	return t, ok
}

// Exists checks if a tool with the specified name is registered.
func (r *ToolRegistry) Exists(name string) bool {
	_, ok := r.Find(name)
	return ok
}

// All returns a copy of all registered tools.
// The returned slice can be safely modified without affecting the registry.
func (r *ToolRegistry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.store))
	for _, t := range r.store {
		tools = append(tools, t)
	}
	return tools
}

// Names returns a copy of all registered tool names.
// The returned slice can be safely modified without affecting the registry.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.store))
	for name := range r.store {
		names = append(names, name)
	}
	return names
}

// Size returns the total number of registered tools.
func (r *ToolRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.store)
}

// Clear removes all tools from the registry.
// Returns the registry for method chaining.
func (r *ToolRegistry) Clear() *ToolRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()

	clear(r.store)
	return r
}

// ToolInvokeResult represents the outcome of tool invocation in LLM chat interactions.
// It encapsulates execution results and determines the next steps in conversation flow.
type ToolInvokeResult struct {
	request           *Request     // original LLM chat request
	response          *Response    // LLM response containing tool calls
	toolMessage       *ToolMessage // aggregated responses from internal tools
	returnDirect      bool         // whether ALL internal tools are configured for direct return
	externalToolCalls []*ToolCall  // tool calls requiring external execution
}

// ShouldRequest determines if a new chat request should be constructed
// for continued LLM processing. Returns true ONLY when:
// - No external tools exist (external tools always return directly) AND
// - At least one internal tool is configured for LLM integration (returnDirect=false)
func (r *ToolInvokeResult) ShouldRequest() bool {
	// External tools always return directly - no LLM continuation
	if len(r.externalToolCalls) > 0 {
		return false
	}

	// For internal tools only: continue with LLM if any tool requires integration
	return !r.returnDirect
}

// ShouldResponse determines if a chat response should be constructed
// for direct return to client. This is the inverse of ShouldRequest.
func (r *ToolInvokeResult) ShouldResponse() bool {
	return !r.ShouldRequest()
}

// MakeRequest constructs a new chat request for continued LLM processing.
// This integrates tool responses into conversation history and prepares the request
// for the next LLM interaction cycle.
//
// Called ONLY when no external tools exist and at least one internal tool
// requires LLM integration (returnDirect=false).
func (r *ToolInvokeResult) MakeRequest() (*Request, error) {
	if !r.ShouldRequest() {
		return nil, errors.New("cannot make chat request")
	}
	if r.request == nil {
		return nil, errors.New("original chat request is required")
	}
	if r.response == nil {
		return nil, errors.New("chat response is required")
	}
	if r.toolMessage == nil {
		return nil, errors.New("tool response message is required")
	}

	res := r.response.firstToolCallsResult()
	if res == nil {
		return nil, errors.New("tool calls result is required")
	}

	history := r.request.Messages
	msgs := slices.Clone(history)
	msgs = append(msgs, res.AssistantMessage)
	msgs = append(msgs, r.toolMessage)

	req, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	req.Options = r.request.Options
	return req, nil
}

// MakeResponse constructs a chat response for direct return to client.
// This creates a response that either contains external tool calls for client-side
// execution or provides direct results when ALL internal tools are configured
// for direct return.
func (r *ToolInvokeResult) MakeResponse() (*Response, error) {
	if !r.ShouldResponse() {
		return nil, errors.New("cannot make chat response")
	}
	if r.response == nil {
		return nil, errors.New("chat response is required")
	}

	res := r.response.firstToolCallsResult()
	if res == nil {
		return nil, errors.New("tool calls result is required")
	}

	msg := res.AssistantMessage
	newMsg := NewAssistantMessage(
		MessageParams{
			Text:      msg.Text,
			Media:     msg.Media,
			ToolCalls: r.externalToolCalls,
			Metadata:  msg.Metadata,
		})

	newRes, err := NewResult(newMsg, res.Metadata)
	if err != nil {
		return nil, err
	}
	newRes.ToolMessage = r.toolMessage

	return NewResponse([]*Result{newRes}, r.response.Metadata)
}

func validateInvokeResult(result *ToolInvokeResult) error {
	if result.request == nil {
		return errors.New("original chat request is required")
	}
	if result.response == nil {
		return errors.New("chat response is required")
	}
	if result.toolMessage == nil && len(result.externalToolCalls) == 0 {
		return errors.New("tool response or external tool calls is required")
	}
	return nil
}

// toolInvoker handles the execution of tool calls from LLM responses.
// It processes both internal tools (executed immediately) and external tools
// (delegated to client), managing flow control and result aggregation.
type toolInvoker struct {
	registry *ToolRegistry // tool registry for looking up available tools
}

// newToolInvoker creates a new tool invoker with the specified registry.
func newToolInvoker(registry *ToolRegistry) *toolInvoker {
	return &toolInvoker{
		registry: registry,
	}
}

// canInvokeToolCalls determines if the chat response contains valid tool calls
// that can be processed. It validates that all requested tools exist in the registry.
func (i *toolInvoker) canInvokeToolCalls(response *Response) (bool, error) {
	res := response.firstToolCallsResult()
	if res == nil {
		return false, nil
	}

	for _, call := range res.AssistantMessage.ToolCalls {
		_, ok := i.registry.Find(call.Name)
		if !ok {
			return false, fmt.Errorf("tool not found: %s", call.Name)
		}
		return true, nil
	}

	return false, nil
}

// createContext creates a new execution context for tool operations.
func (i *toolInvoker) createContext(ctx context.Context, request *Request) *ToolContext {
	toolCtx := NewToolContext(ctx)
	if request.Options == nil {
		return toolCtx
	}

	if opts, ok := request.Options.(ToolOptions); ok {
		return toolCtx.SetMap(opts.ToolParams())
	}

	return toolCtx
}

// invokeToolCalls processes a list of tool calls, executing internal tools immediately
// and collecting external tools for client-side processing.
func (i *toolInvoker) invokeToolCalls(ctx *ToolContext, toolCalls []*ToolCall) (*ToolInvokeResult, error) {
	var (
		extCalls     []*ToolCall   // tools requiring external execution
		returnDirect = true        // whether to return results directly
		responses    []*ToolReturn // responses from internal tools
	)

	for _, call := range toolCalls {
		// Tool existence guaranteed by canInvokeToolCalls precheck
		t, _ := i.registry.Find(call.Name)

		ct, ok := t.(CallableTool)
		if !ok {
			// External tool - add to delegation list
			extCalls = append(extCalls, call)
			continue
		}

		// Internal tool - call immediately
		result, err := ct.Call(ctx, call.Arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to call tool %s: %w", call.Name, err)
		}

		// Update flow control based on tool metadata
		returnDirect = returnDirect && ct.Metadata().ReturnDirect
		responses = append(responses, &ToolReturn{
			ID:     call.ID,
			Name:   call.Name,
			Result: result,
		})
	}

	// Create tool response message from internal tool results
	toolMsg, err := NewToolMessage(responses)
	if err != nil {
		return nil, err
	}

	return &ToolInvokeResult{
		toolMessage:       toolMsg,
		externalToolCalls: extCalls,
		returnDirect:      returnDirect,
	}, nil
}

// invoke orchestrates the complete tool invocation process for an LLM chat interaction.
// It validates tool calls, executes available tools, and constructs the appropriate
// result for the next step in the conversation flow.
func (i *toolInvoker) invoke(ctx context.Context, request *Request, response *Response) (*ToolInvokeResult, error) {
	canInvoke, err := i.canInvokeToolCalls(response)
	if err != nil {
		return nil, err
	}
	if !canInvoke {
		return nil, errors.New("no valid tool calls to invoke")
	}

	// Tool calls result guaranteed by canInvokeToolCalls precheck
	res := response.firstToolCallsResult()

	invokeRes, err := i.invokeToolCalls(
		i.createContext(ctx, request),
		res.AssistantMessage.ToolCalls,
	)
	if err != nil {
		return nil, err
	}

	invokeRes.request = request
	invokeRes.response = response

	return invokeRes, validateInvokeResult(invokeRes)
}

// ToolSupport provides a high-level interface for managing tools and processing tool calls
// in LLM chat interactions. It combines tool registry management with tool invocation
// capabilities for common tool-related operations.
type ToolSupport struct {
	registry *ToolRegistry // tool registry for managing tool instances
	invoker  *toolInvoker  // tool invocation processor
}

// NewToolSupport creates a new ToolSupport instance with an internal tool registry.
// The optional cap parameter specifies the initial capacity for the tool registry.
//
// Example:
//
//	helper := NewToolSupport()       // Default capacity
//	helper := NewToolSupport(50)     // Initial capacity of 50 tools
func NewToolSupport(cap ...int) *ToolSupport {
	registry := newToolRegistry(cap...)
	return &ToolSupport{
		registry: registry,
		invoker:  newToolInvoker(registry),
	}
}

// Registry returns the internal tool registry for direct tool management operations.
func (h *ToolSupport) Registry() *ToolRegistry {
	return h.registry
}

// RegisterTools registers multiple tools to the internal registry.
func (h *ToolSupport) RegisterTools(tools ...Tool) {
	h.registry.Register(tools...)
}

// UnregisterTools removes tools by name from the registry.
func (h *ToolSupport) UnregisterTools(names ...string) {
	h.registry.Unregister(names...)
}

// ShouldReturnDirect determines if a conversation should return directly to the user
// based on the last message in the conversation history.
//
// Returns true when:
// - The last message is a ToolMessage AND
// - ALL tools referenced in the message are registered AND
// - ALL tools are configured with returnDirect=true
func (h *ToolSupport) ShouldReturnDirect(msgs []Message) bool {
	// Check if the last message is a tool response
	if !hasMessageTypeAtLast(msgs, MessageTypeTool) {
		return false
	}

	msg, _ := pkgSlices.Last(msgs)
	toolMsg, ok := msg.(*ToolMessage)
	if !ok {
		return false
	}

	returnDirect := true
	for _, resp := range toolMsg.ToolReturns {
		// Verify tool exists in registry
		t, ok := h.registry.Find(resp.Name)
		if !ok {
			return false // Unknown tool - cannot determine behavior
		}

		// ALL tools must be configured for direct return
		returnDirect = returnDirect && t.Metadata().ReturnDirect
	}

	return returnDirect
}

// MakeReturnDirectResponse creates a chat response for direct return when all tools
// are configured for direct return.
func (h *ToolSupport) MakeReturnDirectResponse(msgs []Message) (*Response, error) {
	if !h.ShouldReturnDirect(msgs) {
		return nil, errors.New("cannot create return direct chat response")
	}

	msg, _ := pkgSlices.Last(msgs)

	assistantMsg := NewAssistantMessage(map[string]any{
		"create_by": FinishReasonReturnDirect.String(),
	})

	meta := &ResultMetadata{
		FinishReason: FinishReasonReturnDirect,
	}

	res, err := NewResult(assistantMsg, meta)
	if err != nil {
		return nil, err
	}
	// prechecked by ShouldReturnDirect
	res.ToolMessage = msg.(*ToolMessage)

	return NewResponse([]*Result{res}, &ResponseMetadata{})
}

// ShouldInvokeToolCalls determines if the chat response contains valid tool calls
// that should be processed. It validates that all requested tools exist in the registry.
func (h *ToolSupport) ShouldInvokeToolCalls(response *Response) (bool, error) {
	return h.invoker.canInvokeToolCalls(response)
}

// InvokeToolCalls processes tool calls from an LLM chat response, executing internal tools
// immediately and preparing external tools for client-side execution.
//
// The method orchestrates the complete tool invocation workflow:
// 1. Validates that tool calls should be processed
// 2. Separates internal tools (CallableTool) from external tools
// 3. Executes internal tools and collects their responses
// 4. Determines conversation flow control based on tool metadata and external tool presence
// 5. Constructs InvokeResult with appropriate configuration for next steps
//
// Flow control logic:
// - External tools always force direct return
// - Internal tools: if ANY tool has returnDirect=false, continue with LLM processing
// - Only when ALL internal tools have returnDirect=true does the flow bypass LLM
func (h *ToolSupport) InvokeToolCalls(ctx context.Context, request *Request, response *Response) (*ToolInvokeResult, error) {
	return h.invoker.invoke(ctx, request, response)
}
