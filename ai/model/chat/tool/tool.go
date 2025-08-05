package tool

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/pkg/assert"
)

// Tool represents an immutable tool definition that can be invoked by LLM models.
// Once created, tools cannot be modified, ensuring consistent behavior across
// LLM interactions and maintaining thread safety in concurrent environments.
//
// Design Philosophy - Internal vs External Execution:
// Supports two distinct execution patterns based on immutable tool configuration:
//
// 1. External Tools: Immutable tools requiring client-side execution
//   - Examples: user interactions, UI updates, file operations, notifications
//   - Only implement Tool interface (no Call method)
//   - Framework delegates execution to external environment
//   - Results are always returned directly to user (returnDirect setting ignored)
//
// 2. Internal Tools: Immutable tools with built-in execution capability
//   - Examples: database queries, calculations, web searches, data processing
//   - Implement both Tool and CallableTool interfaces
//   - Framework executes directly via immutable Call method reference
//   - Usually configured with returnDirect=false for LLM integration
//
// Benefits of immutable design:
// - Thread-safe concurrent LLM interactions
// - Consistent tool behavior throughout application lifecycle
// - Safe sharing across multiple LLM instances
// - Compile-time guarantees about tool capabilities
type Tool interface {
	// Definition returns the immutable tool definition for LLM recognition.
	// Contains unchangeable tool metadata including name, description, and parameter schema
	// that guides LLM decision-making throughout the tool's lifetime.
	Definition() *Definition

	// Metadata returns the immutable execution configuration.
	// Defines unchangeable behavior settings such as result handling patterns
	// that remain consistent across all tool invocations.
	Metadata() *Metadata
}

// CallableTool extends Tool with immutable internal execution capability.
// Tools implementing this interface contain an immutable execution function
// that provides consistent behavior across all invocations.
//
// The immutable Call method reference ensures:
// - Consistent execution behavior throughout tool lifetime
// - Thread-safe concurrent invocations
// - Predictable resource utilization patterns
// - Compile-time execution capability guarantees
//
// Once created, the execution logic cannot be modified, providing stability
// for long-running LLM applications and shared tool registries.
type CallableTool interface {
	Tool

	// Call executes the tool's immutable business logic within the framework.
	// The execution function is fixed at creation time and cannot be changed,
	// ensuring consistent behavior across all invocations.
	//
	// Parameters:
	//   - ctx: Execution context with conversation state and environment info
	//   - input: Input parameters (typically JSON format)
	//
	// Returns:
	//   - string: Tool execution result for LLM processing
	//   - error: Execution error if the immutable logic fails
	Call(ctx Context, input string) (string, error)
}

// tool provides immutable base implementation for external tools.
// Once created, the definition and metadata cannot be changed, ensuring
// consistent delegation behavior throughout the tool's lifetime.
type tool struct {
	definition *Definition // Immutable tool definition
	metadata   *Metadata   // Immutable execution metadata
}

// Definition returns the immutable tool definition.
func (t *tool) Definition() *Definition {
	return t.definition
}

// Metadata returns the immutable execution metadata.
func (t *tool) Metadata() *Metadata {
	return t.metadata
}

// callableTool provides immutable implementation for internal tools.
// Combines immutable base tool properties with a fixed execution function
// that cannot be modified after creation, ensuring consistent execution behavior.
type callableTool struct {
	tool
	caller func(ctx Context, input string) (string, error) // Immutable execution function
}

// Call executes the tool's immutable business logic.
// The caller function is fixed at creation time and cannot be changed.
func (t *callableTool) Call(ctx Context, input string) (string, error) {
	if t.caller == nil {
		return "", fmt.Errorf("caller function is required for internal tool %s", t.definition.Name())
	}
	return t.caller(ctx, input)
}

// Builder provides a fluent interface for constructing immutable Tool instances.
// Once Build() is called, the resulting tool cannot be modified, ensuring
// consistent behavior throughout its lifetime.
//
// Automatically determines tool type based on configuration:
// - With caller function: Creates immutable CallableTool (internal execution)
// - Without caller: Creates immutable Tool (external delegation)
//
// The builder ensures all tools are properly validated before becoming immutable.
type Builder struct {
	definition *Definition
	metadata   *Metadata
	caller     func(ctx Context, input string) (string, error)
}

// NewBuilder creates a new builder for constructing immutable tools.
// All configuration is validated before creating the final immutable instance.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithDefinition sets the immutable tool definition if not nil.
// Once the tool is built, this definition cannot be changed.
// Required for all tools - build will fail without a valid definition.
//
// Parameters:
//   - definition: Immutable definition containing tool metadata for LLM recognition
//
// Returns:
//   - *Builder: Builder instance for method chaining
func (b *Builder) WithDefinition(definition *Definition) *Builder {
	if definition != nil {
		b.definition = definition
	}
	return b
}

// WithMetadata sets the immutable execution metadata if not nil.
// Once the tool is built, this metadata cannot be changed.
// If not provided, defaults to returnDirect=false for LLM integration.
//
// Parameters:
//   - metadata: Immutable execution configuration
//
// Returns:
//   - *Builder: Builder instance for method chaining
func (b *Builder) WithMetadata(metadata *Metadata) *Builder {
	if metadata != nil {
		b.metadata = metadata
	}
	return b
}

// WithCaller sets the immutable execution function for internal tools.
// Once the tool is built, this execution logic cannot be changed.
// Providing a caller creates a CallableTool with internal execution capability.
//
// The caller function becomes permanently associated with the tool,
// ensuring consistent execution behavior throughout its lifetime.
//
// Parameters:
//   - caller: Immutable function implementing the tool's business logic
//
// Returns:
//   - *Builder: Builder instance for method chaining
func (b *Builder) WithCaller(caller func(ctx Context, input string) (string, error)) *Builder {
	if caller != nil {
		b.caller = caller
	}
	return b
}

// validate ensures all required components are present before creating immutable tool.
// Applies default metadata if not explicitly provided.
//
// Validation requirements:
//   - definition must not be nil
//   - definition name must not be empty
//   - metadata defaults to NewMetadata(false) if not provided
//
// Returns:
//   - error: Validation error if requirements not met
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

// Build creates an immutable Tool instance with the configured parameters.
// Once created, the tool cannot be modified, ensuring consistent behavior
// throughout its lifetime and thread safety across concurrent LLM interactions.
//
// Tool type determination:
// - With caller: Creates immutable CallableTool for internal execution
// - Without caller: Creates immutable Tool for external delegation
//
// The resulting tool is thread-safe and can be safely shared across
// multiple LLM instances and concurrent operations.
//
// Returns:
//   - Tool: Immutable tool instance (external or internal based on configuration)
//   - error: Validation error if required components are missing
func (b *Builder) Build() (Tool, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	baseTool := tool{
		definition: b.definition,
		metadata:   b.metadata,
	}

	if b.caller == nil {
		// External tool: immutable Tool interface implementation
		return &baseTool, nil
	}

	// Internal tool: immutable CallableTool implementation
	return &callableTool{
		tool:   baseTool,
		caller: b.caller,
	}, nil
}

// MustBuild creates an immutable Tool instance, panicking on validation failure.
// The resulting tool cannot be modified and is thread-safe for concurrent use.
//
// Recommended for:
// - Application initialization where all parameters are known valid
// - Static tool definitions where errors are unexpected
// - Test scenarios where panicking is acceptable
//
// The created tool maintains immutable behavior throughout its lifetime.
//
// Panics:
//   - If validation fails due to missing required components
//
// Returns:
//   - Tool: Immutable tool instance ready for LLM integration
func (b *Builder) MustBuild() Tool {
	return assert.ErrorIsNil(b.Build())
}
