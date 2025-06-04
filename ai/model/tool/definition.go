package tool

import (
	"errors"
	"strings"

	pkgString "github.com/Tangerg/lynx/pkg/strings"
)

// Definition represents a tool definition that is used by the AI model to determine
// when and how to call the tool. It provides essential metadata including the tool's
// name, description, and input parameter schema.
type Definition interface {
	// Name returns the tool name. The name must be unique within the tool set
	// provided to a model and should follow naming conventions (e.g., camelCase
	// or snake_case). This name is used by the AI model to identify and invoke
	// the specific tool.
	Name() string

	// Description returns the tool description, which is used by the AI model
	// to understand what the tool does and when it should be called. A clear
	// and concise description helps the model make better decisions about
	// tool usage.
	Description() string

	// InputSchema returns the JSON Schema that defines the structure and
	// validation rules for the parameters used to call the tool. This schema
	// helps the AI model understand what arguments to provide when invoking
	// the tool.
	InputSchema() string
}

// definition is the default implementation of the Definition interface.
// It provides a simple, immutable representation of a tool definition
// with all required fields.
type definition struct {
	name        string // The unique tool name
	description string // The tool description for AI model guidance
	inputSchema string // The JSON Schema for tool parameters
}

func (d *definition) Name() string {
	return d.name
}

func (d *definition) Description() string {
	return d.description
}

func (d *definition) InputSchema() string {
	return d.inputSchema
}

// DefinitionBuilder provides a fluent interface for constructing Definition instances.
// It supports method chaining and offers features like automatic description
// generation and validation of required fields.
type DefinitionBuilder struct {
	name     string
	desc     string
	schema   string
	autoDesc bool
}

// NewDefinitionBuilder creates and returns a new DefinitionBuilder instance with default values.
// All fields are initially empty and auto-description is disabled by default.
func NewDefinitionBuilder() *DefinitionBuilder {
	return &DefinitionBuilder{}
}

// WithName sets the tool name for the definition being built.
// The name parameter must be unique within the tool set provided to a model
// and should not be empty. The name is used by the AI model to identify
// and invoke the specific tool.
//
// Parameters:
//   - name: The unique identifier for the tool
//
// Returns:
//   - *DefinitionBuilder: The builder instance for method chaining
func (b *DefinitionBuilder) WithName(name string) *DefinitionBuilder {
	b.name = name
	return b
}

// WithDescription sets the tool description for the definition being built.
// The description should clearly explain what the tool does and when it
// should be used. This information helps the AI model make informed
// decisions about tool invocation.
//
// Parameters:
//   - desc: A clear and concise description of the tool's functionality
//
// Returns:
//   - *DefinitionBuilder: The builder instance for method chaining
func (b *DefinitionBuilder) WithDescription(desc string) *DefinitionBuilder {
	b.desc = desc
	return b
}

// WithAutoDescription enables automatic description generation based on the tool name.
// When enabled, if no explicit description is provided via WithDescription(),
// the builder will automatically generate a description by converting the tool
// name from camelCase to a human-readable format and appending "tool".
//
// For example:
//   - "getUserInfo" becomes "get user info tool"
//   - "calculateSum" becomes "calculate sum tool"
//
// Returns:
//   - *DefinitionBuilder: The builder instance for method chaining
func (b *DefinitionBuilder) WithAutoDescription() *DefinitionBuilder {
	b.autoDesc = true
	return b
}

// WithInputSchema sets the JSON Schema that defines the structure and validation
// rules for the tool's input parameters. The schema should follow the JSON Schema
// specification and describe all required and optional parameters, their types,
// and any validation constraints.
//
// Example schema:
//
//	{
//	  "type": "object",
//	  "properties": {
//	    "city": {"type": "string", "description": "The city name"},
//	    "units": {"type": "string", "enum": ["celsius", "fahrenheit"]}
//	  },
//	  "required": ["city"]
//	}
//
// Parameters:
//   - schema: A valid JSON Schema string defining the tool's input parameters
//
// Returns:
//   - *DefinitionBuilder: The builder instance for method chaining
func (b *DefinitionBuilder) WithInputSchema(schema string) *DefinitionBuilder {
	b.schema = schema
	return b
}

// validate performs validation checks on the builder's current state and
// generates a description if auto-description is enabled and no explicit
// description was provided. This method is called internally before building
// the final Definition instance.
//
// Validation rules:
//   - name must not be empty
//   - inputSchema must not be empty
//   - if autoDesc is true and desc is empty, generates a default description
//
// Returns:
//   - error: An error if validation fails, nil otherwise
func (b *DefinitionBuilder) validate() error {
	if b.name == "" {
		return errors.New("name is required")
	}
	if b.schema == "" {
		return errors.New("input schema is required")
	}

	// Generate description if auto-description is enabled and no explicit description is set
	if b.desc == "" && b.autoDesc {
		b.desc = b.generateDescription()
	}

	return nil
}

// generateDescription creates a default description based on the tool name.
// It performs the following transformations:
//  1. Converts the name to camelCase then to snake_case
//  2. Replaces underscores with spaces
//  3. Trims any existing "tool" prefix or suffix to avoid duplication
//  4. Appends " tool" to the result
//
// Examples:
//   - "getUserInfo" -> "get user info tool"
//   - "tool_calculate_sum" -> "calculate sum tool"
//   - "weatherTool" -> "weather tool"
//
// Returns:
//   - string: The generated description, or "tool" if the name is empty
func (b *DefinitionBuilder) generateDescription() string {
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

// Build creates and returns a new Definition instance with the configured parameters.
// This method performs validation to ensure all required fields are properly set
// and generates auto-description if enabled.
//
// The method will return an error if:
//   - name is empty or not set
//   - inputSchema is empty or not set
//
// Returns:
//   - Definition: A new Definition instance if validation passes
//   - error: An error describing what validation failed, nil on success
func (b *DefinitionBuilder) Build() (Definition, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	return &definition{
		name:        b.name,
		description: b.desc,
		inputSchema: b.schema,
	}, nil
}

// MustBuild creates and returns a new Definition instance, panicking if validation fails.
// This is a convenience method for cases where errors are not expected or when
// building definitions during application initialization where panicking is acceptable.
//
// Use this method when:
//   - You are confident that all required fields are properly set
//   - You are building definitions during application startup
//   - You prefer to fail fast rather than handle errors
//
// Panics:
//   - If validation fails (e.g., missing required fields)
//
// Returns:
//   - Definition: A new Definition instance
func (b *DefinitionBuilder) MustBuild() Definition {
	def, err := b.Build()
	if err != nil {
		panic(err)
	}
	return def
}
