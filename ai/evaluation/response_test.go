package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponse_EnsureMetadata(t *testing.T) {
	t.Run("initialize nil metadata", func(t *testing.T) {
		resp := &Response{}
		assert.Nil(t, resp.Metadata)

		resp.ensureMetadata()
		assert.NotNil(t, resp.Metadata)
		assert.Empty(t, resp.Metadata)
	})

	t.Run("preserve existing metadata", func(t *testing.T) {
		resp := &Response{
			Metadata: map[string]any{
				"key": "value",
			},
		}

		resp.ensureMetadata()
		assert.Equal(t, "value", resp.Metadata["key"])
	})
}

func TestResponse_Get(t *testing.T) {
	t.Run("get existing key", func(t *testing.T) {
		resp := &Response{
			Metadata: map[string]any{
				"key1": "value1",
				"key2": 42,
			},
		}

		value, ok := resp.Get("key1")
		assert.True(t, ok)
		assert.Equal(t, "value1", value)

		value, ok = resp.Get("key2")
		assert.True(t, ok)
		assert.Equal(t, 42, value)
	})

	t.Run("get non-existing key", func(t *testing.T) {
		resp := &Response{
			Metadata: map[string]any{
				"key1": "value1",
			},
		}

		value, ok := resp.Get("non_existing")
		assert.False(t, ok)
		assert.Nil(t, value)
	})

	t.Run("get from nil metadata", func(t *testing.T) {
		resp := &Response{}

		value, ok := resp.Get("key")
		assert.False(t, ok)
		assert.Nil(t, value)
		assert.NotNil(t, resp.Metadata)
	})
}

func TestResponse_Set(t *testing.T) {
	t.Run("set on nil metadata", func(t *testing.T) {
		resp := &Response{}

		resp.Set("key", "value")
		assert.NotNil(t, resp.Metadata)
		assert.Equal(t, "value", resp.Metadata["key"])
	})

	t.Run("set on existing metadata", func(t *testing.T) {
		resp := &Response{
			Metadata: map[string]any{
				"existing": "old",
			},
		}

		resp.Set("new", "value")
		assert.Equal(t, "old", resp.Metadata["existing"])
		assert.Equal(t, "value", resp.Metadata["new"])
	})

	t.Run("overwrite existing key", func(t *testing.T) {
		resp := &Response{
			Metadata: map[string]any{
				"key": "old_value",
			},
		}

		resp.Set("key", "new_value")
		assert.Equal(t, "new_value", resp.Metadata["key"])
	})

	t.Run("set different types", func(t *testing.T) {
		resp := &Response{}

		resp.Set("string", "text")
		resp.Set("int", 123)
		resp.Set("float", 45.67)
		resp.Set("bool", true)
		resp.Set("slice", []string{"a", "b"})

		assert.Equal(t, "text", resp.Metadata["string"])
		assert.Equal(t, 123, resp.Metadata["int"])
		assert.Equal(t, 45.67, resp.Metadata["float"])
		assert.Equal(t, true, resp.Metadata["bool"])
		assert.Equal(t, []string{"a", "b"}, resp.Metadata["slice"])
	})
}

func TestBuildResponse(t *testing.T) {
	t.Run("build response with YES", func(t *testing.T) {
		resp, err := buildResponse("YES")
		require.NoError(t, err)
		assert.True(t, resp.Pass)
		assert.Equal(t, 1.0, resp.Score)
	})

	t.Run("build response with yes lowercase", func(t *testing.T) {
		resp, err := buildResponse("yes")
		require.NoError(t, err)
		assert.True(t, resp.Pass)
		assert.Equal(t, 1.0, resp.Score)
	})

	t.Run("build response with Yes mixed case", func(t *testing.T) {
		resp, err := buildResponse("Yes")
		require.NoError(t, err)
		assert.True(t, resp.Pass)
		assert.Equal(t, 1.0, resp.Score)
	})

	t.Run("build response with NO", func(t *testing.T) {
		resp, err := buildResponse("NO")
		require.NoError(t, err)
		assert.False(t, resp.Pass)
		assert.Equal(t, 0.0, resp.Score)
	})

	t.Run("build response with other text", func(t *testing.T) {
		resp, err := buildResponse("maybe")
		require.NoError(t, err)
		assert.False(t, resp.Pass)
		assert.Equal(t, 0.0, resp.Score)
	})

	t.Run("build response with empty string", func(t *testing.T) {
		resp, err := buildResponse("")
		require.NoError(t, err)
		assert.False(t, resp.Pass)
		assert.Equal(t, 0.0, resp.Score)
	})
}

func TestMergeResponses(t *testing.T) {
	t.Run("merge empty responses", func(t *testing.T) {
		_, err := mergeResponses([]*Response{})
		assert.Error(t, err)
		assert.Equal(t, "empty responses", err.Error())
	})

	t.Run("merge single response", func(t *testing.T) {
		resp := &Response{
			Pass:     true,
			Score:    0.8,
			Feedback: "Good response",
			Metadata: map[string]any{"key": "value"},
		}

		merged, err := mergeResponses([]*Response{resp})
		require.NoError(t, err)
		assert.Equal(t, resp, merged)
	})

	t.Run("merge all passing responses", func(t *testing.T) {
		resp1 := &Response{
			Pass:     true,
			Score:    0.9,
			Feedback: "Excellent",
			Metadata: map[string]any{"metric1": 10},
		}
		resp2 := &Response{
			Pass:     true,
			Score:    0.8,
			Feedback: "Good",
			Metadata: map[string]any{"metric2": 20},
		}

		merged, err := mergeResponses([]*Response{resp1, resp2})
		require.NoError(t, err)

		assert.True(t, merged.Pass)
		assert.Equal(t, int(0.85*100), int(merged.Score*100))
		assert.Contains(t, merged.Feedback, "[Evaluation 1] Excellent")
		assert.Contains(t, merged.Feedback, "[Evaluation 2] Good")
		assert.Equal(t, 2, merged.Metadata["total_evaluations"])
		assert.Equal(t, 2, merged.Metadata["passed_count"])
		assert.Equal(t, 10, merged.Metadata["eval_1_metric1"])
		assert.Equal(t, 20, merged.Metadata["eval_2_metric2"])
	})

	t.Run("merge with one failing response", func(t *testing.T) {
		resp1 := &Response{
			Pass:     true,
			Score:    1.0,
			Feedback: "Pass",
		}
		resp2 := &Response{
			Pass:     false,
			Score:    0.0,
			Feedback: "Fail",
		}

		merged, err := mergeResponses([]*Response{resp1, resp2})
		require.NoError(t, err)

		assert.False(t, merged.Pass)
		assert.Equal(t, 0.5, merged.Score)
		assert.Equal(t, 2, merged.Metadata["total_evaluations"])
		assert.Equal(t, 1, merged.Metadata["passed_count"])
	})

	t.Run("merge all failing responses", func(t *testing.T) {
		resp1 := &Response{
			Pass:  false,
			Score: 0.3,
		}
		resp2 := &Response{
			Pass:  false,
			Score: 0.2,
		}
		resp3 := &Response{
			Pass:  false,
			Score: 0.1,
		}

		merged, err := mergeResponses([]*Response{resp1, resp2, resp3})
		require.NoError(t, err)

		assert.False(t, merged.Pass)
		assert.InDelta(t, 0.2, merged.Score, 0.01)
		assert.Equal(t, 3, merged.Metadata["total_evaluations"])
		assert.Equal(t, 0, merged.Metadata["passed_count"])
	})

	t.Run("merge responses with empty feedback", func(t *testing.T) {
		resp1 := &Response{
			Pass:     true,
			Score:    0.9,
			Feedback: "",
		}
		resp2 := &Response{
			Pass:     true,
			Score:    0.8,
			Feedback: "Has feedback",
		}

		merged, err := mergeResponses([]*Response{resp1, resp2})
		require.NoError(t, err)

		assert.Equal(t, "[Evaluation 2] Has feedback", merged.Feedback)
	})

	t.Run("merge responses with complex metadata", func(t *testing.T) {
		resp1 := &Response{
			Pass:  true,
			Score: 0.7,
			Metadata: map[string]any{
				"accuracy":  0.95,
				"precision": 0.88,
			},
		}
		resp2 := &Response{
			Pass:  true,
			Score: 0.6,
			Metadata: map[string]any{
				"recall":   0.92,
				"f1_score": 0.90,
			},
		}

		merged, err := mergeResponses([]*Response{resp1, resp2})
		require.NoError(t, err)

		assert.Equal(t, 0.95, merged.Metadata["eval_1_accuracy"])
		assert.Equal(t, 0.88, merged.Metadata["eval_1_precision"])
		assert.Equal(t, 0.92, merged.Metadata["eval_2_recall"])
		assert.Equal(t, 0.90, merged.Metadata["eval_2_f1_score"])
	})

	t.Run("merge multiple responses with varied scores", func(t *testing.T) {
		responses := []*Response{
			{Pass: true, Score: 1.0, Feedback: "Perfect"},
			{Pass: true, Score: 0.8, Feedback: "Good"},
			{Pass: false, Score: 0.4, Feedback: "Poor"},
			{Pass: false, Score: 0.2, Feedback: "Bad"},
		}

		merged, err := mergeResponses(responses)
		require.NoError(t, err)

		assert.False(t, merged.Pass)
		assert.Equal(t, int(0.6*10), int(merged.Score*10))
		assert.Equal(t, 4, merged.Metadata["total_evaluations"])
		assert.Equal(t, 2, merged.Metadata["passed_count"])
		assert.Contains(t, merged.Feedback, "[Evaluation 1] Perfect")
		assert.Contains(t, merged.Feedback, "[Evaluation 2] Good")
		assert.Contains(t, merged.Feedback, "[Evaluation 3] Poor")
		assert.Contains(t, merged.Feedback, "[Evaluation 4] Bad")
	})
}
