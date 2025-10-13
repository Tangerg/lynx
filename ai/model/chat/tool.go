package chat

import (
	"context"
	"errors"
	"fmt"
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
	Call(ctx context.Context, arguments string) (string, error)
}

// baseTool provides the base implementation for external tools requiring delegation.
type baseTool struct {
	definition ToolDefinition
	metadata   ToolMetadata
}

func (t *baseTool) Definition() ToolDefinition {
	return t.definition
}

func (t *baseTool) Metadata() ToolMetadata {
	return t.metadata
}

// callableTool provides the implementation for internal tools with execution capability.
// Combines base properties with an execution function.
type callableTool struct {
	baseTool
	execFunc func(ctx context.Context, arguments string) (string, error)
}

func (t *callableTool) Call(ctx context.Context, arguments string) (string, error) {
	if t.execFunc == nil {
		return "", fmt.Errorf("execution function is required for tool %s", t.definition.Name)
	}

	return t.execFunc(ctx, arguments)
}

// NewTool creates a new tool instance.
// If execFunc is provided, returns a CallableTool; otherwise returns a Tool for external execution.
//
// Parameters:
//   - definition: Tool metadata and schema information
//   - metadata: Execution behavior configuration
//   - execFunc: Optional call function (nil for external tools)
//
// Returns:
//   - Tool: External tool (when execFunc is nil) or CallableTool (when callFunc is provided)
//   - error: Validation error if required fields are missing
func NewTool(definition ToolDefinition, metadata ToolMetadata, execFunc func(ctx context.Context, arguments string) (string, error)) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("tool name cannot be empty")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("tool input schema cannot be empty")
	}

	base := baseTool{
		definition: definition,
		metadata:   metadata,
	}

	if execFunc == nil {
		return &base, nil
	}

	return &callableTool{
		baseTool: base,
		execFunc: execFunc,
	}, nil
}

// ToolRegistry provides thread-safe management of immutable tool instances for LLM applications.
// Uses tool names as unique identifiers and prevents duplicate registrations.
// All operations are concurrent-safe and work with immutable tools that cannot be modified after creation.
type ToolRegistry struct {
	mu    sync.RWMutex    // Protects concurrent access to the tools map
	tools map[string]Tool // Maps tool names to immutable Tool instances
}

// newToolRegistry creates a new registry with optional initial capacity.
// Negative capacity values default to 0.
func newToolRegistry(capacityHint ...int) *ToolRegistry {
	capacity, _ := pkgSlices.First(capacityHint)
	if capacity < 0 {
		capacity = 0
	}

	return &ToolRegistry{
		tools: make(map[string]Tool, capacity),
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
		delete(r.tools, name)
	}

	return r
}

// Find retrieves a tool by name.
// Returns the tool and true if found, nil and false otherwise.
func (r *ToolRegistry) Find(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// Exists checks if a tool with the specified name is registered.
func (r *ToolRegistry) Exists(name string) bool {
	_, exists := r.Find(name)
	return exists
}

// All returns a copy of all registered tools.
// The returned slice can be safely modified without affecting the registry.
func (r *ToolRegistry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}

	return result
}

// Names returns a copy of all registered tool names.
// The returned slice can be safely modified without affecting the registry.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.tools))
	for name := range r.tools {
		result = append(result, name)
	}

	return result
}

// Size returns the total number of registered tools.
func (r *ToolRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// Clear removes all tools from the registry.
// Returns the registry for method chaining.
func (r *ToolRegistry) Clear() *ToolRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()

	clear(r.tools)

	return r
}

// ToolInvocationResult represents the outcome of tool invocation in LLM chat interactions.
// It encapsulates execution results and determines the next steps in conversation flow.
//
// The result handles two types of tools:
//   - Internal tools: execution results are captured in toolMessage
//   - External tools: invocation requests are collected in externalToolCalls for client-side execution
type ToolInvocationResult struct {
	request           *Request     // original LLM chat request
	response          *Response    // LLM response containing tool calls
	toolMessage       *ToolMessage // aggregated responses from internal tools
	allReturnDirect   bool         // whether ALL internal tools are configured for direct return
	externalToolCalls []*ToolCall  // tool calls requiring external execution
}

// ShouldContinue determines if a new chat request should be constructed
// for continued LLM processing. Returns true ONLY when:
// - No external tools exist (external tools always return directly) AND
// - At least one internal tool is configured for LLM integration (returnDirect=false)
func (r *ToolInvocationResult) ShouldContinue() bool {
	// External tools always return directly - no LLM continuation
	if len(r.externalToolCalls) > 0 {
		return false
	}

	// For internal tools only: continue with LLM if any tool requires integration
	return !r.allReturnDirect
}

// ShouldReturn determines if a chat response should be constructed
// for direct return to client. This is the inverse of ShouldContinue.
func (r *ToolInvocationResult) ShouldReturn() bool {
	return !r.ShouldContinue()
}

// BuildContinueRequest appends tool invocation results to the original chat request
// for continued LLM processing. This method modifies the original request in-place by
// adding the assistant's tool call message and the tool execution results to the
// conversation history, preparing it for the next LLM interaction cycle.
//
// Called ONLY when no external tools exist and at least one internal tool
// requires LLM integration (returnDirect=false).
//
// Note: This method mutates the original request by appending messages to its Messages slice.
func (r *ToolInvocationResult) BuildContinueRequest() (*Request, error) {
	if !r.ShouldContinue() {
		return nil, errors.New("cannot build continuation request: should return directly")
	}
	if r.request == nil {
		return nil, errors.New("original chat request is required")
	}
	if r.response == nil {
		return nil, errors.New("LLM response is required")
	}
	if r.toolMessage == nil {
		return nil, errors.New("internal tools message is required")
	}

	result := r.response.findFirstResultWithToolCalls()
	if result == nil {
		return nil, errors.New("result with tool calls is required")
	}

	r.request.Messages = append(r.request.Messages, result.AssistantMessage)
	r.request.Messages = append(r.request.Messages, r.toolMessage)

	return r.request, nil
}

// BuildReturnResponse constructs a chat response for direct return to client.
// This creates a response that either contains external tool calls for client-side
// execution or provides direct results when ALL internal tools are configured
// for direct return.
func (r *ToolInvocationResult) BuildReturnResponse() (*Response, error) {
	if !r.ShouldReturn() {
		return nil, errors.New("cannot build direct response: should continue with LLM")
	}
	if r.response == nil {
		return nil, errors.New("LLM response is required")
	}

	result := r.response.findFirstResultWithToolCalls()
	if result == nil {
		return nil, errors.New("result with tool calls is required")
	}

	origMsg := result.AssistantMessage

	modMsg := NewAssistantMessage(
		MessageParams{
			Text:      origMsg.Text,
			Media:     origMsg.Media,
			ToolCalls: r.externalToolCalls,
			Metadata:  origMsg.Metadata,
		})

	modResult, err := NewResult(modMsg, result.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to create modified result: %w", err)
	}

	modResult.ToolMessage = r.toolMessage

	return NewResponse([]*Result{modResult}, r.response.Metadata)
}

func (r *ToolInvocationResult) validate() error {
	if r.request == nil {
		return errors.New("original chat request is required")
	}
	if r.response == nil {
		return errors.New("LLM response is required")
	}
	if r.toolMessage == nil && len(r.externalToolCalls) == 0 {
		return errors.New("either internal tools message or external tool calls is required")
	}

	return nil
}

// toolCallInvoker handles the invocation of tool calls from LLM responses.
// It processes both internal tools (executed immediately) and external tools
// (delegated to client), managing flow control and result aggregation.
type toolCallInvoker struct {
	registry *ToolRegistry // tool registry for looking up available tools
}

// newToolCallInvoker creates a new tool call invoker with the specified registry.
func newToolCallInvoker(registry *ToolRegistry) *toolCallInvoker {
	return &toolCallInvoker{
		registry: registry,
	}
}

// canInvokeToolCalls determines if the chat response contains valid tool calls
// that can be processed. It validates that all requested tools exist in the registry.
func (i *toolCallInvoker) canInvokeToolCalls(resp *Response) (bool, error) {
	result := resp.findFirstResultWithToolCalls()
	if result == nil {
		return false, nil
	}

	for _, toolCall := range result.AssistantMessage.ToolCalls {
		_, exists := i.registry.Find(toolCall.Name)
		if !exists {
			return false, fmt.Errorf("tool not found in registry: %s", toolCall.Name)
		}
	}

	return true, nil
}

// invokeToolCalls processes a list of tool calls, executing internal tools immediately
// and collecting external tools for client-side processing.
func (i *toolCallInvoker) invokeToolCalls(ctx context.Context, toolCalls []*ToolCall) (*ToolInvocationResult, error) {
	var (
		externalCalls   []*ToolCall   // tools requiring external execution
		allReturnDirect = true        // whether to return results directly
		internalReturns []*ToolReturn // responses from internal tools
	)

	for _, toolCall := range toolCalls {
		// Tool existence guaranteed by canInvokeToolCalls precheck
		tool, _ := i.registry.Find(toolCall.Name)

		callable, ok := tool.(CallableTool)
		if !ok {
			// External tool - add to delegation list
			externalCalls = append(externalCalls, toolCall)
			continue
		}

		// Internal tool - execute immediately
		result, err := callable.Call(ctx, toolCall.Arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to execute tool %s: %w", toolCall.Name, err)
		}

		// Update flow control based on tool metadata
		allReturnDirect = allReturnDirect && callable.Metadata().ReturnDirect

		internalReturns = append(internalReturns, &ToolReturn{
			ID:     toolCall.ID,
			Name:   toolCall.Name,
			Result: result,
		})
	}

	// Create tool response message from internal tool results
	toolMsg, err := NewToolMessage(internalReturns)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal tools message: %w", err)
	}

	return &ToolInvocationResult{
		toolMessage:       toolMsg,
		externalToolCalls: externalCalls,
		allReturnDirect:   allReturnDirect,
	}, nil
}

// invoke orchestrates the complete tool invocation process for an LLM chat interaction.
// It validates tool calls, invokes available tools, and constructs the appropriate
// result for the next step in the conversation flow.
func (i *toolCallInvoker) invoke(ctx context.Context, req *Request, resp *Response) (*ToolInvocationResult, error) {
	canInvoke, err := i.canInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !canInvoke {
		return nil, errors.New("no valid tool calls to invoke")
	}

	// Tool calls result guaranteed by canInvokeToolCalls precheck
	result := resp.findFirstResultWithToolCalls()

	invResult, err := i.invokeToolCalls(ctx, result.AssistantMessage.ToolCalls)
	if err != nil {
		return nil, err
	}

	invResult.request = req
	invResult.response = resp

	return invResult, invResult.validate()
}

// ToolSupport provides a high-level interface for managing tools and processing tool calls
// in LLM chat interactions. It combines tool registry management with tool invocation
// capabilities for common tool-related operations.
type ToolSupport struct {
	registry *ToolRegistry    // tool registry for managing tool instances
	invoker  *toolCallInvoker // tool invocation processor
}

// NewToolSupport creates a new ToolSupport instance with an internal tool registry.
// The optional capacityHint parameter specifies the initial capacity for the tool registry.
//
// Example:
//
//	support := NewToolSupport()       // Default capacity
//	support := NewToolSupport(50)     // Initial capacity of 50 tools
func NewToolSupport(capacityHint ...int) *ToolSupport {
	registry := newToolRegistry(capacityHint...)

	return &ToolSupport{
		registry: registry,
		invoker:  newToolCallInvoker(registry),
	}
}

// Registry returns the internal tool registry for direct tool management operations.
func (s *ToolSupport) Registry() *ToolRegistry {
	return s.registry
}

// RegisterTools registers multiple tools to the internal registry.
func (s *ToolSupport) RegisterTools(tools ...Tool) {
	s.registry.Register(tools...)
}

// UnregisterTools removes tools by name from the registry.
func (s *ToolSupport) UnregisterTools(names ...string) {
	s.registry.Unregister(names...)
}

// ShouldReturnDirect determines if a conversation should return directly to the user
// based on the last message in the conversation history.
//
// Returns true when:
// - The last message is a ToolMessage AND
// - ALL tools referenced in the message are registered AND
// - ALL tools are configured with returnDirect=true
func (s *ToolSupport) ShouldReturnDirect(msgs []Message) bool {
	// Check if the last message is a tool response
	if !hasMessageTypeAtLast(msgs, MessageTypeTool) {
		return false
	}

	lastMsg, _ := pkgSlices.Last(msgs)
	toolMsg, ok := lastMsg.(*ToolMessage)
	if !ok {
		return false
	}

	allDirect := true
	for _, toolReturn := range toolMsg.ToolReturns {
		// Verify tool exists in registry
		tool, exists := s.registry.Find(toolReturn.Name)
		if !exists {
			return false // Unknown tool - cannot determine behavior
		}

		// ALL tools must be configured for direct return
		allDirect = allDirect && tool.Metadata().ReturnDirect
	}

	return allDirect
}

// BuildReturnDirectResponse creates a chat response for direct return when all tools
// are configured for direct return.
func (s *ToolSupport) BuildReturnDirectResponse(msgs []Message) (*Response, error) {
	if !s.ShouldReturnDirect(msgs) {
		return nil, errors.New("cannot build return direct response: conditions not met")
	}

	lastMsg, _ := pkgSlices.Last(msgs)

	assistantMsg := NewAssistantMessage(map[string]any{
		"created_by": FinishReasonReturnDirect.String(),
	})

	metadata := &ResultMetadata{
		FinishReason: FinishReasonReturnDirect,
	}

	result, err := NewResult(assistantMsg, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to create result: %w", err)
	}

	// Prechecked by ShouldReturnDirect
	result.ToolMessage = lastMsg.(*ToolMessage)

	return NewResponse([]*Result{result}, &ResponseMetadata{})
}

// ShouldInvokeToolCalls determines if the chat response contains valid tool calls
// that should be processed. It validates that all requested tools exist in the registry.
func (s *ToolSupport) ShouldInvokeToolCalls(resp *Response) (bool, error) {
	return s.invoker.canInvokeToolCalls(resp)
}

// InvokeToolCalls processes tool calls from an LLM chat response, executing internal tools
// immediately and preparing external tools for client-side execution.
//
// The method orchestrates the complete tool invocation workflow:
// 1. Validates that tool calls should be processed
// 2. Separates internal tools (CallableTool) from external tools
// 3. Executes internal tools and collects their responses
// 4. Determines conversation flow control based on tool metadata and external tool presence
// 5. Constructs InvocationResult with appropriate configuration for next steps
//
// Flow control logic:
// - External tools always force direct return
// - Internal tools: if ANY tool has returnDirect=false, continue with LLM processing
// - Only when ALL internal tools have returnDirect=true does the flow bypass LLM
func (s *ToolSupport) InvokeToolCalls(ctx context.Context, req *Request, resp *Response) (*ToolInvocationResult, error) {
	return s.invoker.invoke(ctx, req, resp)
}
