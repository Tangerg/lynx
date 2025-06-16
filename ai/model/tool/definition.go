package tool

import (
	"errors"
	"github.com/Tangerg/lynx/pkg/assert"
	"strings"

	pkgString "github.com/Tangerg/lynx/pkg/strings"
)

// Definition represents an immutable tool definition used by LLM models to understand
// when and how to invoke external functions. Once created, the definition cannot be modified,
// ensuring consistency and thread safety across concurrent LLM interactions.
//
// Contains essential metadata for LLM tool calling:
//   - Unique tool identifier for LLM recognition
//   - Human-readable description for LLM decision making
//   - JSON Schema defining input parameter structure
type Definition struct {
	name        string // Immutable unique tool identifier
	description string // Immutable tool description for LLM guidance
	inputSchema string // Immutable JSON Schema for input validation
}

// Name returns the immutable tool identifier.
// This name must be unique within the tool set and follows LLM provider naming conventions.
// The LLM uses this identifier to reference and invoke the specific tool.
func (d *Definition) Name() string {
	return d.name
}

// Description returns the immutable tool description.
// Provides context to help the LLM understand the tool's purpose and appropriate usage scenarios.
// A clear description improves LLM tool selection accuracy.
func (d *Definition) Description() string {
	return d.description
}

// InputSchema returns the immutable JSON Schema for tool parameters.
// Defines the structure, types, and validation rules for arguments the LLM should provide
// when invoking this tool. Must be valid JSON Schema format.
func (d *Definition) InputSchema() string {
	return d.inputSchema
}

// DefinitionBuilder provides a fluent interface for constructing immutable Definition instances.
// Supports method chaining, automatic description generation, and validation before creation.
// Once Build() is called, the resulting Definition cannot be modified.
type DefinitionBuilder struct {
	name            string
	description     string
	inputSchema     string
	autoDescription bool
}

// NewDefinitionBuilder creates a new builder for constructing immutable tool definitions.
// All fields start empty and can be configured through method chaining.
func NewDefinitionBuilder() *DefinitionBuilder {
	return &DefinitionBuilder{}
}

// WithName sets the unique tool identifier if the name is not empty.
// The name becomes immutable once the Definition is built.
// Must be unique within the LLM's tool set and follow provider naming conventions.
//
// Parameters:
//   - name: Unique identifier for LLM tool recognition
//
// Returns:
//   - *DefinitionBuilder: Builder instance for method chaining
func (b *DefinitionBuilder) WithName(name string) *DefinitionBuilder {
	if name != "" {
		b.name = name
	}
	return b
}

// WithDescription sets the tool description if not empty.
// The description becomes immutable once the Definition is built.
// Should clearly explain the tool's functionality to help LLM decision making.
//
// Parameters:
//   - description: Clear explanation of tool functionality for LLM understanding
//
// Returns:
//   - *DefinitionBuilder: Builder instance for method chaining
func (b *DefinitionBuilder) WithDescription(desc string) *DefinitionBuilder {
	if desc != "" {
		b.description = desc
	}
	return b
}

// WithAutoDescription enables automatic description generation from the tool name.
// If no explicit description is provided, generates a human-readable description
// by converting camelCase names to readable format with "tool" suffix.
//
// Examples:
//   - "getUserInfo" → "get user info tool"
//   - "calculateSum" → "calculate sum tool"
//
// The generated description becomes immutable once the Definition is built.
//
// Returns:
//   - *DefinitionBuilder: Builder instance for method chaining
func (b *DefinitionBuilder) WithAutoDescription() *DefinitionBuilder {
	b.autoDescription = true
	return b
}

// WithInputSchema sets the JSON Schema for tool parameters if not empty.
// The schema becomes immutable once the Definition is built.
// Must be valid JSON Schema defining parameter structure and validation rules.
//
// Example schema:
//
//	{
//	  "type": "object",
//	  "properties": {
//	    "city": {"type": "string", "description": "City name"},
//	    "units": {"type": "string", "enum": ["celsius", "fahrenheit"]}
//	  },
//	  "required": ["city"]
//	}
//
// Parameters:
//   - schema: Valid JSON Schema string for parameter validation
//
// Returns:
//   - *DefinitionBuilder: Builder instance for method chaining
func (b *DefinitionBuilder) WithInputSchema(schema string) *DefinitionBuilder {
	if schema != "" {
		b.inputSchema = schema
	}
	return b
}

// validate ensures all required fields are set before creating an immutable Definition.
// Generates automatic description if enabled and no explicit description was provided.
//
// Required fields:
//   - name: Must not be empty
//   - inputSchema: Must not be empty
//   - description: Auto-generated if autoDescription is enabled and description is empty
//
// Returns:
//   - error: Validation error if required fields are missing, nil if valid
func (b *DefinitionBuilder) validate() error {
	if b.name == "" {
		return errors.New("name is required")
	}
	if b.inputSchema == "" {
		return errors.New("input schema is required")
	}

	// Generate description if auto-description is enabled and no explicit description is set
	if b.description == "" && b.autoDescription {
		b.description = b.genDescription()
	}

	return nil
}

// genDescription automatically generates a tool description from the tool name.
// Converts camelCase to readable format and adds "tool" suffix.
//
// Transformation process:
//  1. Convert name to camelCase then to snake_case
//  2. Replace underscores with spaces
//  3. Remove existing "tool" prefix/suffix to avoid duplication
//  4. Append " tool" to the result
//
// Examples:
//   - "getUserInfo" → "get user info tool"
//   - "weatherTool" → "weather tool"
//
// Returns:
//   - string: Generated description, or "tool" if name is empty
func (b *DefinitionBuilder) genDescription() string {
	if b.name == "" {
		return "tool"
	}

	desc := pkgString.AsCamelCase(b.name).ToSnakeCase().String()
	desc = strings.ReplaceAll(desc, "_", " ")
	desc = strings.TrimSpace(desc)

	desc = strings.TrimPrefix(desc, "tool ")
	desc = strings.TrimSuffix(desc, " tool")

	if desc == "" {
		return "tool"
	}

	return desc + " tool"
}

// Build creates an immutable Definition instance after validation.
// Once created, the Definition cannot be modified, ensuring thread safety
// and consistency across LLM interactions.
//
// Validation requirements:
//   - name must not be empty
//   - inputSchema must not be empty
//
// Returns:
//   - *Definition: Immutable tool definition if validation passes
//   - error: Validation error if required fields are missing
func (b *DefinitionBuilder) Build() (*Definition, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	return &Definition{
		name:        b.name,
		description: b.description,
		inputSchema: b.inputSchema,
	}, nil
}

// MustBuild creates an immutable Definition instance, panicking on validation failure.
// Use when errors are unexpected or during application initialization where
// failing fast is preferred over error handling.
//
// The resulting Definition is immutable and thread-safe.
//
// Recommended usage:
//   - Application startup configuration
//   - Static tool definitions where all fields are known to be valid
//   - Test scenarios where panicking on errors is acceptable
//
// Panics:
//   - If validation fails due to missing required fields
//
// Returns:
//   - *Definition: Immutable tool definition
func (b *DefinitionBuilder) MustBuild() *Definition {
	return assert.ErrorIsNil(b.Build())
}
