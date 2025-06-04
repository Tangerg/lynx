package tool

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/model"
)

// Tool represents a tool whose execution can be triggered by an AI model.
// It combines a definition (for the AI model to understand its capabilities),
// metadata (for execution configuration), and an executor function (for actual execution).
type Tool interface {
	// Definition returns the tool definition used by the AI model to determine
	// when and how to call the tool. This includes the tool's name, description,
	// and input parameter schema.
	Definition() Definition

	// Metadata returns metadata providing additional information on how to handle
	// the tool execution, such as whether results should be returned directly
	// or passed back to the model for further processing.
	Metadata() Metadata

	// Call executes the tool with the given context and input parameters.
	// The context may contain additional information from the conversation or
	// execution environment, while input contains the specific parameters for
	// this tool invocation.
	//
	// Parameters:
	//   - ctx: model.ToolContext The tool execution context containing conversation state and environment info
	//   - input: The input parameters as a string (typically JSON format)
	//
	// Returns:
	//   - string: The tool execution result to be sent back to the AI model
	//   - error: An error if the tool execution fails
	Call(ctx *model.ToolContext, input string) (string, error)
}

// tool is the default implementation of the Tool interface.
// It provides a simple, immutable representation of a tool with
// its definition, metadata, and executor function.
type tool struct {
	definition Definition
	metadata   Metadata
	executor   func(ctx *model.ToolContext, input string) (string, error)
}

// Definition returns the tool definition.
func (t *tool) Definition() Definition {
	return t.definition
}

// Metadata returns the tool metadata.
func (t *tool) Metadata() Metadata {
	return t.metadata
}

// Call executes the tool by invoking the configured executor function.
func (t *tool) Call(ctx *model.ToolContext, input string) (string, error) {
	return t.executor(ctx, input)
}

// Builder provides a fluent interface for constructing Tool instances.
// It allows step-by-step configuration of tool components and validates
// that all required components are provided before building the final Tool.
type Builder struct {
	definition Definition
	metadata   Metadata
	executor   func(ctx *model.ToolContext, input string) (string, error)
}

// NewBuilder creates and returns a new Builder instance with default values.
// All fields are initially nil and will be validated during the build process.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithDefinition sets the tool definition for the tool being built if def is not nil.
// The definition contains essential information that the AI model uses to
// understand the tool's purpose, parameters, and when to invoke it.
//
// Parameters:
//   - def: A Definition instance containing tool name, description, and input schema
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithDefinition(def Definition) *Builder {
	if def != nil {
		b.definition = def
	}
	return b
}

// WithMetadata sets the tool metadata for the tool being built if metadata is not nil.
// Metadata provides additional configuration for tool execution, such as
// whether results should be returned directly to the user or passed back
// to the AI model for further processing.
//
// If not set, default metadata will be applied during validation.
//
// Parameters:
//   - metadata: A Metadata instance containing execution configuration
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithMetadata(metadata Metadata) *Builder {
	if metadata != nil {
		b.metadata = metadata
	}
	return b
}

// WithExecutor sets the executor function for the tool being built if executor is not nil.
// The executor function contains the actual business logic that will be
// executed when the AI model invokes this tool. It receives the execution
// context and input parameters, and returns the result or an error.
//
// The executor function signature:
//   - ctx: Tool execution context with conversation state and environment info
//   - input: Input parameters as a string (typically JSON format)
//   - Returns: (result string, error)
//
// Parameters:
//   - executor: The function that implements the tool's business logic
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithExecutor(executor func(ctx *model.ToolContext, input string) (string, error)) *Builder {
	if executor != nil {
		b.executor = executor
	}
	return b
}

// validate performs validation checks on the builder's current state and
// applies default values where appropriate. This method ensures that all
// required components are properly configured before building the Tool.
//
// Validation rules:
//   - definition must not be nil (required)
//   - executor must not be nil (required)
//   - metadata will be set to default if nil (optional)
//
// Default values applied:
//   - metadata: NewMetadata(false) - creates metadata with returnDirect=false
//
// Returns:
//   - error: An error describing what validation failed, nil if validation passes
func (b *Builder) validate() error {
	if b.definition == nil {
		return errors.New("tool definition is required")
	}
	if b.metadata == nil {
		b.metadata = NewMetadata(false)
	}
	if b.executor == nil {
		return errors.New("tool executor is required")
	}
	return nil
}

// Build creates and returns a new Tool instance with the configured parameters.
// This method performs validation to ensure all required components are properly
// set and applies default values where appropriate.
//
// The build process:
//  1. Validates that required components (definition, executor) are set
//  2. Applies default metadata if not explicitly provided
//  3. Creates and returns the immutable Tool instance
//
// Returns:
//   - Tool: A new Tool instance if validation passes
//   - error: An error if validation fails (e.g., missing required components)
func (b *Builder) Build() (Tool, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	return &tool{
		definition: b.definition,
		metadata:   b.metadata,
		executor:   b.executor,
	}, nil
}

// MustBuild creates and returns a new Tool instance, panicking if validation fails.
// This is a convenience method for cases where errors are not expected or when
// building tools during application initialization where panicking is acceptable.
//
// Use this method when:
//   - You are confident that all required components are properly set
//   - You are building tools during application startup where failing fast is preferred
//   - You prefer to avoid explicit error handling in favor of fail-fast behavior
//
// Panics:
//   - If validation fails (e.g., missing definition or executor)
//
// Returns:
//   - Tool: A new Tool instance
func (b *Builder) MustBuild() Tool {
	t, err := b.Build()
	if err != nil {
		panic(err)
	}
	return t
}
