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

func TestRelevancyEvaluatorConfig_Validate(t *testing.T) {
	t.Run("validate nil config", func(t *testing.T) {
		var config *RelevancyEvaluatorConfig
		err := config.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil Config")
	})

	t.Run("validate nil chat model", func(t *testing.T) {
		config := &RelevancyEvaluatorConfig{}
		err := config.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil ChatModel")
	})

	t.Run("validate with chat model only", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}
		err := config.validate()
		require.NoError(t, err)
		assert.NotNil(t, config.PromptTemplate)
	})

	t.Run("validate with custom template missing variables", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate("Invalid template")

		config := &RelevancyEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}
		err := config.validate()
		assert.Error(t, err)
	})

	t.Run("validate with valid custom template", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate(
			"Query: {{.Query}}\nResponse: {{.Response}}\nContext: {{.Context}}",
		)

		config := &RelevancyEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}
		err := config.validate()
		require.NoError(t, err)
	})
}

func TestNewRelevancyEvaluatorConfig(t *testing.T) {
	t.Run("create with nil config", func(t *testing.T) {
		evaluator, err := NewRelevancyEvaluatorConfig(nil)
		assert.Error(t, err)
		assert.Nil(t, evaluator)
	})

	t.Run("create with invalid config", func(t *testing.T) {
		config := &RelevancyEvaluatorConfig{}
		evaluator, err := NewRelevancyEvaluatorConfig(config)
		assert.Error(t, err)
		assert.Nil(t, evaluator)
	})

	t.Run("create with valid config", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)
		assert.NotNil(t, evaluator)
		assert.NotNil(t, evaluator.chatClient)
		assert.NotNil(t, evaluator.promptTemplate)
	})

	t.Run("create with custom template", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		template := chat.NewPromptTemplate().WithTemplate(
			"Custom template: {{.Query}} {{.Response}} {{.Context}}",
		)

		config := &RelevancyEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)
		assert.Equal(t, template, evaluator.promptTemplate)
	})
}

func TestRelevancyEvaluator_Evaluate(t *testing.T) {
	t.Run("evaluate with nil request", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "nil request")
	})

	t.Run("evaluate relevant response", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The capital of France is Paris. Paris is known for the Eiffel Tower.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is the capital of France?",
			Generation: "The capital of France is Paris.",
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

	t.Run("evaluate irrelevant response", func(t *testing.T) {
		chatModel := newTestChatModel(t)
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The capital of France is Paris.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is the capital of France?",
			Generation: "The capital of France is London.",
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
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		doc1, err := document.NewDocument(
			"Artificial Intelligence is a branch of computer science.",
			nil,
		)
		require.NoError(t, err)

		doc2, err := document.NewDocument(
			"AI involves creating machines that can perform tasks requiring human intelligence.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is AI?",
			Generation: "AI is a branch of computer science that creates intelligent machines.",
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
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is AI?",
			Generation: "AI is artificial intelligence.",
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
		config := &RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		doc, err := document.NewDocument("Some context", nil)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "Test prompt",
			Generation: "Test generation",
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
			`Evaluate relevancy. Answer YES or NO only.
			Query: {{.Query}}
			Response: {{.Response}}
			Context: {{.Context}}
			Answer:`,
		)

		config := &RelevancyEvaluatorConfig{
			ChatModel:      chatModel,
			PromptTemplate: template,
		}

		evaluator, err := NewRelevancyEvaluatorConfig(config)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"Python is a programming language.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What is Python?",
			Generation: "Python is a programming language.",
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
