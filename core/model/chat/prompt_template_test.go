package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media"
)

func TestNewPromptTemplate(t *testing.T) {
	pt := NewPromptTemplate()

	assert.NotNil(t, pt)
	assert.NotNil(t, pt.renderer)
	assert.NotNil(t, pt.media)
	assert.Empty(t, pt.media)
}

func TestPromptTemplate_WithTemplate(t *testing.T) {
	pt := NewPromptTemplate()
	template := "Hello {{.name}}"

	result := pt.WithTemplate(template)

	assert.Equal(t, pt, result)
}

func TestPromptTemplate_WithVariable(t *testing.T) {
	tests := []struct {
		name     string
		template string
		varName  string
		varValue any
		expected string
	}{
		{
			name:     "string variable",
			template: "Hello {{.name}}",
			varName:  "name",
			varValue: "World",
			expected: "Hello World",
		},
		{
			name:     "integer variable",
			template: "Count: {{.count}}",
			varName:  "count",
			varValue: 42,
			expected: "Count: 42",
		},
		{
			name:     "boolean variable",
			template: "Active: {{.active}}",
			varName:  "active",
			varValue: true,
			expected: "Active: true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewPromptTemplate().
				WithTemplate(tt.template).
				WithVariable(tt.varName, tt.varValue)

			rendered, err := pt.Render()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, rendered)
		})
	}
}

func TestPromptTemplate_WithVariables(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("Hello {{.name}}, you are {{.age}} years old").
		WithVariables(map[string]any{
			"name": "Alice",
			"age":  30,
		})

	rendered, err := pt.Render()
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice, you are 30 years old", rendered)
}

func TestPromptTemplate_WithVariables_ChainMultiple(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("{{.a}} {{.b}} {{.c}}").
		WithVariables(map[string]any{
			"a": "1",
			"b": "2",
			"c": "3",
		})

	rendered, err := pt.Render()
	require.NoError(t, err)
	assert.Equal(t, "1 2 3", rendered)
}

func TestPromptTemplate_WithMedia(t *testing.T) {
	media1 := &media.Media{}
	media2 := &media.Media{}

	pt := NewPromptTemplate().WithMedia(media1, media2)

	assert.Len(t, pt.media, 2)
	assert.Equal(t, media1, pt.media[0])
	assert.Equal(t, media2, pt.media[1])
}

func TestPromptTemplate_WithMedia_Multiple(t *testing.T) {
	media1 := &media.Media{}
	media2 := &media.Media{}
	media3 := &media.Media{}

	pt := NewPromptTemplate().
		WithMedia(media1).
		WithMedia(media2, media3)

	assert.Len(t, pt.media, 3)
}

func TestPromptTemplate_WithMedia_Empty(t *testing.T) {
	pt := NewPromptTemplate().WithMedia()

	assert.Empty(t, pt.media)
}

func TestPromptTemplate_RequireVariables(t *testing.T) {
	tests := []struct {
		name         string
		template     string
		requiredVars []string
		shouldError  bool
	}{
		{
			name:         "all variables exist",
			template:     "{{.name}} {{.age}}",
			requiredVars: []string{"name", "age"},
			shouldError:  false,
		},
		{
			name:         "missing variable",
			template:     "{{.name}}",
			requiredVars: []string{"name", "age"},
			shouldError:  true,
		},
		{
			name:         "no required variables",
			template:     "{{.name}}",
			requiredVars: []string{},
			shouldError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewPromptTemplate().WithTemplate(tt.template)
			err := pt.RequireVariables(tt.requiredVars...)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPromptTemplate_Clone(t *testing.T) {
	media1 := &media.Media{}
	original := NewPromptTemplate().
		WithTemplate("Hello {{.name}}").
		WithVariable("name", "World").
		WithMedia(media1)

	cloned := original.Clone()

	assert.NotNil(t, cloned)
	assert.NotSame(t, original, cloned)
	assert.NotSame(t, original.renderer, cloned.renderer)
	assert.NotSame(t, &original.media, &cloned.media)
	assert.Len(t, cloned.media, 1)

	renderedOriginal, err := original.Render()
	require.NoError(t, err)

	renderedCloned, err := cloned.Render()
	require.NoError(t, err)

	assert.True(t, renderedOriginal == renderedCloned)
}

func TestPromptTemplate_Clone_Nil(t *testing.T) {
	var pt *PromptTemplate
	cloned := pt.Clone()

	assert.Nil(t, cloned)
}

func TestPromptTemplate_Clone_Independence(t *testing.T) {
	original := NewPromptTemplate().
		WithTemplate("Hello {{.name}}").
		WithVariable("name", "Original")

	cloned := original.Clone()
	cloned.WithVariable("name", "Cloned")

	renderedOriginal, err := original.Render()
	require.NoError(t, err)
	assert.Equal(t, "Hello Original", renderedOriginal)

	renderedCloned, err := cloned.Render()
	require.NoError(t, err)
	assert.Equal(t, "Hello Cloned", renderedCloned)
}

func TestPromptTemplate_Render(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		variables   map[string]any
		expected    string
		shouldError bool
	}{
		{
			name:        "simple template",
			template:    "Hello {{.name}}",
			variables:   map[string]any{"name": "World"},
			expected:    "Hello World",
			shouldError: false,
		},
		{
			name:        "multiple variables",
			template:    "{{.greeting}} {{.name}}, you are {{.age}}",
			variables:   map[string]any{"greeting": "Hi", "name": "Alice", "age": 25},
			expected:    "Hi Alice, you are 25",
			shouldError: false,
		},
		{
			name:        "no variables",
			template:    "Static text",
			variables:   map[string]any{},
			expected:    "Static text",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewPromptTemplate().
				WithTemplate(tt.template).
				WithVariables(tt.variables)

			rendered, err := pt.Render()

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, rendered)
			}
		})
	}
}

func TestPromptTemplate_RenderWithVariables(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("{{.a}} {{.b}} {{.c}}").
		WithVariable("a", "1").
		WithVariable("b", "2")

	rendered, err := pt.RenderWithVariables(map[string]any{"c": "3"})
	require.NoError(t, err)
	assert.Equal(t, "1 2 3", rendered)

	_, err = pt.Render()
	require.NoError(t, err)
}

func TestPromptTemplate_RenderWithVariables_Override(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("{{.name}}").
		WithVariable("name", "Original")

	rendered, err := pt.RenderWithVariables(map[string]any{"name": "Override"})
	require.NoError(t, err)
	assert.Equal(t, "Override", rendered)

	renderedOriginal, err := pt.Render()
	require.NoError(t, err)
	assert.Equal(t, "Original", renderedOriginal)
}

func TestPromptTemplate_CreateSystemMessage(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("You are a helpful assistant named {{.name}}").
		WithVariable("name", "AI")

	msg, err := pt.CreateSystemMessage()

	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "You are a helpful assistant named AI", msg.Text)
	assert.Equal(t, MessageTypeSystem, msg.Type())
}

func TestPromptTemplate_CreateSystemMessage_RenderError(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("Hello {{.missing}}")

	msg, err := pt.CreateSystemMessage()

	assert.NoError(t, err)
	assert.NotNil(t, msg)
}

func TestPromptTemplate_CreateUserMessage(t *testing.T) {
	media1 := &media.Media{}
	pt := NewPromptTemplate().
		WithTemplate("Analyze {{.item}}").
		WithVariable("item", "this image").
		WithMedia(media1)

	msg, err := pt.CreateUserMessage()

	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "Analyze this image", msg.Text)
	assert.Equal(t, MessageTypeUser, msg.Type())
	assert.Len(t, msg.Media, 1)
}

func TestPromptTemplate_CreateUserMessage_NoMedia(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("Hello {{.name}}").
		WithVariable("name", "User")

	msg, err := pt.CreateUserMessage()

	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "Hello User", msg.Text)
	assert.Empty(t, msg.Media)
}

func TestPromptTemplate_CreateUserMessage_RenderError(t *testing.T) {
	pt := NewPromptTemplate().
		WithTemplate("Hello {{.missing}}")

	msg, err := pt.CreateUserMessage()

	assert.NoError(t, err)
	assert.NotNil(t, msg)
}

func TestPromptTemplate_MethodChaining(t *testing.T) {
	media1 := &media.Media{}

	pt := NewPromptTemplate().
		WithTemplate("{{.greeting}} {{.name}}").
		WithVariable("greeting", "Hello").
		WithVariable("name", "World").
		WithMedia(media1)

	assert.NotNil(t, pt)

	rendered, err := pt.Render()
	require.NoError(t, err)
	assert.Equal(t, "Hello World", rendered)

	assert.Len(t, pt.media, 1)
}

func TestPromptTemplate_ComplexScenario(t *testing.T) {
	media1 := &media.Media{}
	media2 := &media.Media{}

	baseTemplate := NewPromptTemplate().
		WithTemplate("Analyze {{.subject}} with {{.method}}").
		WithVariable("method", "AI")

	template1 := baseTemplate.Clone().
		WithVariable("subject", "image").
		WithMedia(media1)

	template2 := baseTemplate.Clone().
		WithVariable("subject", "text").
		WithMedia(media2)

	msg1, err := template1.CreateUserMessage()
	require.NoError(t, err)
	assert.Equal(t, "Analyze image with AI", msg1.Text)
	assert.Len(t, msg1.Media, 1)

	msg2, err := template2.CreateUserMessage()
	require.NoError(t, err)
	assert.Equal(t, "Analyze text with AI", msg2.Text)
	assert.Len(t, msg2.Media, 1)
}
