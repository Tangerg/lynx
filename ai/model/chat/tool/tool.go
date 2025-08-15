package tool

import (
	"errors"
	"fmt"
)

// Definition represents a tool definition that enables LLM models to understand
// when and how to invoke external functions.
//
// Contains essential metadata for LLM tool calling:
//   - Name: Unique tool identifier for LLM recognition
//   - Description: Human-readable explanation for LLM decision-making
//   - InputSchema: JSON Schema defining required input parameter structure
type Definition struct {
	Name        string // unique identifier for tool recognition
	Description string // descriptive text guiding LLM usage decisions
	InputSchema string // JSON Schema for parameter validation
}

// Metadata represents execution configuration that controls how the LLM framework
// processes tool results.
type Metadata struct {
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
	Definition() Definition

	// Metadata returns the execution configuration that defines
	// behavior settings for tool invocations.
	Metadata() Metadata
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
	Call(ctx *Context, arguments string) (string, error)
}

// tool provides the base implementation for external tools requiring delegation.
type tool struct {
	definition Definition // tool definition
	metadata   Metadata   // execution configuration
}

// Definition returns the tool definition.
func (t *tool) Definition() Definition {
	return t.definition
}

// Metadata returns the execution configuration.
func (t *tool) Metadata() Metadata {
	return t.metadata
}

// callableTool provides the implementation for internal tools with execution capability.
// Combines base properties with an execution function.
type callableTool struct {
	tool
	caller func(ctx *Context, input string) (string, error) // execution function
}

// Call executes the tool's business logic using the caller function.
func (t *callableTool) Call(ctx *Context, input string) (string, error) {
	if t.caller == nil {
		return "", fmt.Errorf("caller function is required for tool %s", t.definition.Name)
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
func NewTool(definition Definition, metadata Metadata, caller func(ctx *Context, input string) (string, error)) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("tool name is required")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("tool input schema is required")
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
