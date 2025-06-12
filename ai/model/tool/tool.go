package tool

import (
	"errors"
	"fmt"
)

// Tool represents a tool that can be invoked by an AI model during conversation.
//
// Design Philosophy - Internal vs External Execution:
// This interface design supports two distinct execution patterns:
//
// 1. External Tools: Tools that must be executed by the client/user environment
//   - Examples: ask_user, update_view, file_operations, send_notifications
//   - These tools only implement the Tool interface (no Call method)
//   - The framework delegates execution to the external environment
//   - Results are typically returned directly to the user (returnDirect=true)
//
// 2. Internal Tools: Tools that can be executed within the framework
//   - Examples: database_query, web_search, calculations, data_processing
//   - These tools implement both Tool and CallableTool interfaces
//   - The framework executes them directly via the Call method
//   - Results are usually fed back to the AI model for further processing
//
// This separation provides several benefits:
// - Clear responsibility boundaries between client and server
// - Type safety at compile time (CallableTool guarantees Call method exists)
// - Performance optimization (internal tools execute immediately)
// - Security isolation (external tools run in user's environment)
// - Resource management (internal tools can share server resources)
type Tool interface {
	// Definition returns the tool definition used by the AI model to determine
	// when and how to call the tool. This includes the tool's name, description,
	// and input parameter inputSchema that guides the model's decision-making process.
	Definition() *Definition

	// Metadata returns execution metadata that configures how the framework
	// should handle the tool's results, such as whether to return results
	// directly to the user or pass them back to the AI model for further processing.
	Metadata() *Metadata
}

// CallableTool extends Tool to support direct execution within the framework.
//
// Tools implementing this interface are considered "internal tools" that can be
// executed immediately by the framework without external delegation. This design
// enables:
//
// - Immediate execution and response
// - Shared resource utilization (database connections, caches, etc.)
// - Controlled security context
// - Performance optimization through local execution
//
// The presence of the Call method serves as a compile-time indicator that this
// tool can be executed internally. Tools that only implement Tool (without
// CallableTool) are automatically treated as external tools requiring delegation.
//
// Example internal tools: database queries, calculations, web searches, data processing
// Example external tools: user interactions, UI updates, local file operations
type CallableTool interface {
	Tool

	// Call executes the tool's business logic within the framework environment.
	// This method is only called for internal tools and provides immediate
	// execution without external delegation.
	//
	// The execution context contains conversation state and environment information,
	// while input contains the specific parameters for this tool invocation.
	//
	// Parameters:
	//   - ctx: Tool execution context containing conversation state and environment info
	//   - input: Input parameters as a string (typically JSON format)
	//
	// Returns:
	//   - string: The tool execution result, usually fed back to the AI model
	//   - error: An error if the tool execution fails
	Call(ctx Context, input string) (string, error)
}

// tool provides the base implementation of the Tool interface.
// This struct represents external tools that require delegation to the client
// environment for execution. It contains only the essential components needed
// for tool definition and metadata without execution capabilities.
type tool struct {
	definition *Definition
	metadata   *Metadata
}

func (t *tool) Definition() *Definition {
	return t.definition
}

func (t *tool) Metadata() *Metadata {
	return t.metadata
}

// callableTool extends the base tool with internal execution capabilities.
// This struct represents internal tools that can be executed directly within
// the framework. It combines the base tool properties with a caller function
// that implements the actual business logic.
type callableTool struct {
	tool
	caller func(ctx Context, input string) (string, error)
}

func (t *callableTool) Call(ctx Context, input string) (string, error) {
	if t.caller == nil {
		return "", fmt.Errorf("caller function is required for internal tool %s", t.definition.Name())
	}
	return t.caller(ctx, input)
}

// Builder provides a fluent interface for constructing Tool instances with
// proper validation and configuration. It automatically determines whether to
// create an internal tool (with Call capability) or external tool (delegation only)
// based on whether a caller function is provided.
//
// The builder pattern ensures:
// - Required components are validated before construction
// - Appropriate defaults are applied
// - Type-safe tool creation
// - Clear distinction between internal and external tools
type Builder struct {
	definition *Definition
	metadata   *Metadata
	caller     func(ctx Context, input string) (string, error)
}

// NewBuilder creates and returns a new Builder instance for tool construction.
// All fields start as nil and will be validated during the build process.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithDefinition configures the tool definition if the provided definition is not nil.
// The definition contains essential information that the AI model uses to
// understand the tool's purpose, parameters, and when to invoke it.
//
// This is a required component for all tools - the build process will fail
// if no definition is provided.
//
// Parameters:
//   - def: A Definition instance containing tool name, description, and input inputSchema
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithDefinition(def *Definition) *Builder {
	if def != nil {
		b.definition = def
	}
	return b
}

// WithMetadata configures the tool metadata if the provided metadata is not nil.
// Metadata provides execution configuration such as whether results should be
// returned directly to the user or passed back to the AI model.
//
// If not explicitly set, default metadata will be applied during build validation
// with returnDirect=false (results go back to AI model).
//
// Parameters:
//   - metadata: A Metadata instance containing execution configuration
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithMetadata(metadata *Metadata) *Builder {
	if metadata != nil {
		b.metadata = metadata
	}
	return b
}

// WithCaller configures the caller function for internal tool execution.
// Providing a caller function transforms the tool into an internal tool
// (implementing CallableTool) that can be executed directly by the framework.
//
// If no caller is provided, the resulting tool will be an external tool
// that requires delegation to the client environment for execution.
//
// The caller function implements the tool's actual business logic:
//   - ctx: Tool execution context with conversation state and environment info
//   - input: Input parameters as a string (typically JSON format)
//   - Returns: (result string, error)
//
// Parameters:
//   - caller: The function that implements the tool's business logic
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithCaller(caller func(ctx Context, input string) (string, error)) *Builder {
	if caller != nil {
		b.caller = caller
	}
	return b
}

// validate performs comprehensive validation of the builder's configuration
// and applies appropriate defaults. This ensures that the resulting tool
// is properly configured for its intended execution pattern.
//
// Validation rules:
//   - definition must not be nil (required for all tools)
//   - definition name must not be empty
//   - metadata will be set to default if nil
//
// Default values applied:
//   - metadata: NewMetadata(false) - results go back to AI model by default
//
// Returns:
//   - error: An error describing validation failures, nil if validation passes
func (b *Builder) validate() error {
	if b.definition == nil {
		return errors.New("tool definition is required")
	}

	if b.definition.Name() == "" {
		return errors.New("tool definition name cannot be empty")
	}

	if b.metadata == nil {
		b.metadata = NewMetadata(false)
	}

	return nil
}

// Build creates and returns a new Tool instance based on the configured parameters.
// The type of tool created depends on whether a caller function was provided:
//
// - With caller: Returns a CallableTool (internal execution)
// - Without caller: Returns a Tool (external execution via delegation)
//
// This method performs validation to ensure all required components are properly
// configured and applies appropriate defaults where needed.
//
// Build process:
//  1. Validates required components (definition with valid name)
//  2. Applies default metadata if not explicitly provided
//  3. Creates appropriate tool type based on caller presence
//  4. Returns the immutable Tool instance
//
// Returns:
//   - Tool: A new Tool instance (either *tool or *callableTool)
//   - error: An error if validation fails
func (b *Builder) Build() (Tool, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	baseTool := tool{
		definition: b.definition,
		metadata:   b.metadata,
	}

	if b.caller == nil {
		// External tool: only implements Tool interface
		return &baseTool, nil
	}

	// Internal tool: implements both Tool and CallableTool interfaces
	return &callableTool{
		tool:   baseTool,
		caller: b.caller,
	}, nil
}

// MustBuild creates and returns a new Tool instance, panicking if validation fails.
// This is a convenience method for scenarios where:
// - All required components are known to be properly configured
// - Application initialization where failing fast is preferred
// - Error handling complexity should be avoided in favor of fail-fast behavior
//
// Use with caution in production code - prefer Build() for robust error handling.
//
// Panics:
//   - If validation fails (e.g., missing definition, empty tool name)
//
// Returns:
//   - Tool: A new Tool instance (*tool for external, *callableTool for internal)
func (b *Builder) MustBuild() Tool {
	t, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("tool build failed: %v", err))
	}
	return t
}
