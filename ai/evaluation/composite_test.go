package evaluation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
)

type mockEvaluator struct {
	response *Response
	err      error
}

func (m *mockEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestNewCompositeEvaluator(t *testing.T) {
	t.Run("create with empty evaluators", func(t *testing.T) {
		evaluator, err := NewCompositeEvaluator()
		assert.Error(t, err)
		assert.Nil(t, evaluator)
		assert.Contains(t, err.Error(), "empty evaluators")
	})

	t.Run("create with single evaluator", func(t *testing.T) {
		mock := &mockEvaluator{
			response: &Response{Pass: true, Score: 1.0},
		}

		evaluator, err := NewCompositeEvaluator(mock)
		require.NoError(t, err)
		assert.NotNil(t, evaluator)
		assert.Len(t, evaluator.evaluators, 1)
	})

	t.Run("create with multiple evaluators", func(t *testing.T) {
		mock1 := &mockEvaluator{
			response: &Response{Pass: true, Score: 1.0},
		}
		mock2 := &mockEvaluator{
			response: &Response{Pass: true, Score: 0.9},
		}
		mock3 := &mockEvaluator{
			response: &Response{Pass: true, Score: 0.8},
		}

		evaluator, err := NewCompositeEvaluator(mock1, mock2, mock3)
		require.NoError(t, err)
		assert.NotNil(t, evaluator)
		assert.Len(t, evaluator.evaluators, 3)
	})
}

func TestCompositeEvaluator_Evaluate(t *testing.T) {
	t.Run("evaluate with nil request", func(t *testing.T) {
		mock := &mockEvaluator{
			response: &Response{Pass: true, Score: 1.0},
		}

		evaluator, err := NewCompositeEvaluator(mock)
		require.NoError(t, err)

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "nil request")
	})

	t.Run("evaluate with single passing evaluator", func(t *testing.T) {
		mock := &mockEvaluator{
			response: &Response{
				Pass:     true,
				Score:    1.0,
				Feedback: "Excellent",
			},
		}

		evaluator, err := NewCompositeEvaluator(mock)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
		}

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 1.0, response.Score)
		assert.Equal(t, "Excellent", response.Feedback)
	})

	t.Run("evaluate with multiple passing evaluators", func(t *testing.T) {
		mock1 := &mockEvaluator{
			response: &Response{
				Pass:     true,
				Score:    1.0,
				Feedback: "Good relevance",
				Metadata: map[string]any{"metric1": 10},
			},
		}
		mock2 := &mockEvaluator{
			response: &Response{
				Pass:     true,
				Score:    0.9,
				Feedback: "Good accuracy",
				Metadata: map[string]any{"metric2": 20},
			},
		}

		evaluator, err := NewCompositeEvaluator(mock1, mock2)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
		}

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 0.95, response.Score)
		assert.Contains(t, response.Feedback, "[Evaluation 1] Good relevance")
		assert.Contains(t, response.Feedback, "[Evaluation 2] Good accuracy")
		assert.Equal(t, 2, response.Metadata["total_evaluations"])
		assert.Equal(t, 2, response.Metadata["passed_count"])
	})

	t.Run("evaluate with one failing evaluator", func(t *testing.T) {
		mock1 := &mockEvaluator{
			response: &Response{
				Pass:     true,
				Score:    1.0,
				Feedback: "Pass",
			},
		}
		mock2 := &mockEvaluator{
			response: &Response{
				Pass:     false,
				Score:    0.0,
				Feedback: "Fail",
			},
		}

		evaluator, err := NewCompositeEvaluator(mock1, mock2)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
		}

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.False(t, response.Pass)
		assert.Equal(t, 0.5, response.Score)
		assert.Equal(t, 2, response.Metadata["total_evaluations"])
		assert.Equal(t, 1, response.Metadata["passed_count"])
	})

	t.Run("evaluate with all failing evaluators", func(t *testing.T) {
		mock1 := &mockEvaluator{
			response: &Response{
				Pass:     false,
				Score:    0.3,
				Feedback: "Poor relevance",
			},
		}
		mock2 := &mockEvaluator{
			response: &Response{
				Pass:     false,
				Score:    0.2,
				Feedback: "Poor accuracy",
			},
		}
		mock3 := &mockEvaluator{
			response: &Response{
				Pass:     false,
				Score:    0.1,
				Feedback: "Poor coherence",
			},
		}

		evaluator, err := NewCompositeEvaluator(mock1, mock2, mock3)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
		}

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.False(t, response.Pass)
		assert.InDelta(t, 0.2, response.Score, 0.01)
		assert.Equal(t, 3, response.Metadata["total_evaluations"])
		assert.Equal(t, 0, response.Metadata["passed_count"])
	})

	t.Run("evaluate with evaluator error", func(t *testing.T) {
		mock1 := &mockEvaluator{
			response: &Response{Pass: true, Score: 1.0},
		}
		mock2 := &mockEvaluator{
			err: errors.New("evaluation failed"),
		}

		evaluator, err := NewCompositeEvaluator(mock1, mock2)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
		}

		ctx := context.Background()
		response, err := evaluator.Evaluate(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "evaluation failed")
	})

	t.Run("evaluate with context cancellation", func(t *testing.T) {
		mock := &mockEvaluator{
			response: &Response{Pass: true, Score: 1.0},
		}

		evaluator, err := NewCompositeEvaluator(mock)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "test prompt",
			Generation: "test generation",
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = evaluator.Evaluate(ctx, req)
		assert.NoError(t, err)
	})
}

func TestCompositeEvaluator_IntegrationWithRealEvaluators(t *testing.T) {
	t.Run("composite with fact checking and relevancy evaluators", func(t *testing.T) {
		chatModel := newTestChatModel(t)

		factChecker, err := NewFactCheckingEvaluator(&FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		})
		require.NoError(t, err)

		relevancyEvaluator, err := NewRelevancyEvaluatorConfig(&RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		})
		require.NoError(t, err)

		composite, err := NewCompositeEvaluator(factChecker, relevancyEvaluator)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The Moon orbits the Earth. It takes about 27.3 days to complete one orbit.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "How long does the Moon take to orbit Earth?",
			Generation: "The Moon takes about 27.3 days to orbit the Earth.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := composite.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Greater(t, response.Score, 0.0)
		assert.Equal(t, 2, response.Metadata["total_evaluations"])
	})

	t.Run("composite with contradicting evaluators", func(t *testing.T) {
		chatModel := newTestChatModel(t)

		factChecker, err := NewFactCheckingEvaluator(&FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		})
		require.NoError(t, err)

		relevancyEvaluator, err := NewRelevancyEvaluatorConfig(&RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		})
		require.NoError(t, err)

		composite, err := NewCompositeEvaluator(factChecker, relevancyEvaluator)
		require.NoError(t, err)

		doc, err := document.NewDocument(
			"The Moon orbits the Earth.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "What orbits what?",
			Generation: "The Earth orbits the Moon.",
			Documents:  []*document.Document{doc},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := composite.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.False(t, response.Pass)
		assert.Equal(t, 2, response.Metadata["total_evaluations"])
	})

	t.Run("composite with multiple documents", func(t *testing.T) {
		chatModel := newTestChatModel(t)

		factChecker, err := NewFactCheckingEvaluator(&FactCheckingEvaluatorConfig{
			ChatModel: chatModel,
		})
		require.NoError(t, err)

		relevancyEvaluator, err := NewRelevancyEvaluatorConfig(&RelevancyEvaluatorConfig{
			ChatModel: chatModel,
		})
		require.NoError(t, err)

		composite, err := NewCompositeEvaluator(factChecker, relevancyEvaluator)
		require.NoError(t, err)

		doc1, err := document.NewDocument(
			"Venus is the second planet from the Sun.",
			nil,
		)
		require.NoError(t, err)

		doc2, err := document.NewDocument(
			"Venus has a thick atmosphere composed mainly of carbon dioxide.",
			nil,
		)
		require.NoError(t, err)

		req := &Request{
			Prompt:     "Which planet is second from the Sun?",
			Generation: "Venus is the second planet from the Sun.",
			Documents:  []*document.Document{doc1, doc2},
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		response, err := composite.Evaluate(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Pass)
		assert.Equal(t, 2, response.Metadata["total_evaluations"])
	})
}
