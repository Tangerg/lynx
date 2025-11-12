package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/chat"
)

func TestFactCheckingEvaluatorConfig_Validate(t *testing.T) {
	t.Run("validate nil config", func(t *testing.T) {
		var config *FactCheckingEvaluatorConfig
		err := config.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil Config")
	})

	t.Run("validate nil chat model", func(t *testing.T) {
		config := &FactCheckingEvaluatorConfig{}
		err := config.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil ChatModel")
	})

	t.Run("validate with chat model only", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}
		err := config.validate()
		require.NoError(t, err)
		assert.NotNil(t, config.PromptTemplate)
	})

	t.Run("validate with custom template missing variables", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate("Invalid template")

		config := &FactCheckingEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}
		err := config.validate()
		assert.Error(t, err)
	})

	t.Run("validate with valid custom template", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate(
			"Document: {{.Document}}\nClaim: {{.Claim}}",
		)

		config := &FactCheckingEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}
		err := config.validate()
		require.NoError(t, err)
	})
}

func TestNewFactCheckingEvaluator(t *testing.T) {
	t.Run("create with nil config", func(t *testing.T) {
		evaluator, err := NewFactCheckingEvaluator(nil)
		assert.Error(t, err)
		assert.Nil(t, evaluator)
	})

	t.Run("create with invalid config", func(t *testing.T) {
		config := &FactCheckingEvaluatorConfig{}
		evaluator, err := NewFactCheckingEvaluator(config)
		assert.Error(t, err)
		assert.Nil(t, evaluator)
	})

	t.Run("create with valid config", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)
		assert.NotNil(t, evaluator)
		assert.NotNil(t, evaluator.chatClient)
		assert.NotNil(t, evaluator.promptTemplate)
	})

	t.Run("create with custom template", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate(
			"Custom template: {{.Document}} {{.Claim}}",
		)

		config := &FactCheckingEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)
		assert.Equal(t, template, evaluator.promptTemplate)
	})
}

func TestFactCheckingEvaluator_Evaluate(t *testing.T) {
	t.Run("evaluate with nil request", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "nil request")
	})

	t.Run("evaluate supported claim", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The Earth orbits around the Sun. It takes approximately 365.25 days to complete one orbit.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "How long does Earth take to orbit the Sun?",
			Generation: "The Earth takes approximately 365.25 days to orbit the Sun.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 1.0, response.Score)
	})

	t.Run("evaluate unsupported claim", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The Earth orbits around the Sun.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What orbits around what?",
			Generation: "The Sun orbits around the Earth.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.False(t, response.Pass)
		assert.Equal(t, 0.0, response.Score)
	})

	t.Run("evaluate with multiple documents", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc1, err := document.NewDocument(
			"Water freezes at 0 degrees Celsius.",
			nil,
		)
		require.NoError(t, err)

		doc2, err := document.NewDocument(
			"Water boils at 100 degrees Celsius at sea level.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "At what temperature does water freeze?",
			Generation: "Water freezes at 0 degrees Celsius.",
			Documents:  []*document.Document{doc1, doc2},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 1.0, response.Score)
	})

	t.Run("evaluate with empty documents", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is the speed of light?",
			Generation: "The speed of light is 299,792,458 meters per second.",
			Documents:  []*document.Document{},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("evaluate with context timeout", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc, err := document.NewDocument("Some document content", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "Test prompt",
			Generation: "Test claim",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond)

		_, err = evaluator.Evaluate(ctx, req)
		assert.Error(t, err)
	})

	t.Run("evaluate with custom template", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate(
			`Check if claim is supported. Answer YES or NO.
			Document: {{.Document}}
			Claim: {{.Claim}}
			Answer:`,
		)

		config := &FactCheckingEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"Photosynthesis is the process by which plants convert light into energy.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "How do plants produce energy?",
			Generation: "Plants convert light into energy through photosynthesis.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 1.0, response.Score)
	})

	t.Run("evaluate partially supported claim", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The Great Wall of China was built over many centuries.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "Tell me about the Great Wall",
			Generation: "The Great Wall of China was built in one year by a single emperor.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.False(t, response.Pass)
		assert.Equal(t, 0.0, response.Score)
	})

	t.Run("evaluate with complex claim", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewFactCheckingEvaluator(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"Mount Everest is the highest mountain above sea level, with a peak at 8,849 meters. "+
				"It is located in the Himalayas on the border between Nepal and Tibet.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "Where is Mount Everest?",
			Generation: "Mount Everest, the highest mountain at 8,849 meters, is located in the Himalayas.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 1.0, response.Score)
	})
}
