package openai

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/pkg/assert"
)

const (
	testEmbeddingInput        = "test string"
	testEmbeddingInputLong    = "This is a longer test string for embedding model testing with more content"
	testEmbeddingInputChinese = "这是一个中文测试字符串"
)

func newTestEmbeddingModel(t *testing.T) *EmbeddingModel {
	t.Helper()

	if currentConfig.embedModel == "" {
		t.Skip("embed model not configured")
	}

	defaultOptions := assert.Must(embedding.NewOptions(currentConfig.embedModel))

	model, err := NewEmbeddingModel(
		getAPIKey(t),
		defaultOptions,
		option.WithBaseURL(currentConfig.baseURL),
	)
	if err != nil {
		t.Fatalf("failed to create embedding model: %v", err)
	}

	return model
}

func TestNewEmbeddingModel(t *testing.T) {
	if currentConfig.embedModel == "" {
		t.Skip("embed model not configured")
	}

	tests := []struct {
		name           string
		apiKey         model.ApiKey
		defaultOptions *embedding.Options
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "valid configuration",
			apiKey:         getAPIKey(t),
			defaultOptions: assert.Must(embedding.NewOptions(currentConfig.embedModel)),
			wantErr:        false,
		},
		{
			name:           "nil api key",
			apiKey:         nil,
			defaultOptions: assert.Must(embedding.NewOptions(currentConfig.embedModel)),
			wantErr:        true,
			errMsg:         "apiKey is required",
		},
		{
			name:    "nil default options",
			apiKey:  getAPIKey(t),
			wantErr: true,
			errMsg:  "default options cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := NewEmbeddingModel(
				tt.apiKey,
				tt.defaultOptions,
				option.WithBaseURL(currentConfig.baseURL),
			)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error message %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if model == nil {
				t.Error("expected non-nil model")
			}

			if model.defaultOptions == nil {
				t.Error("expected non-nil default options")
			}

			if model.api == nil {
				t.Error("expected non-nil api")
			}
		})
	}
}

func TestEmbeddingModel_Call(t *testing.T) {
	model := newTestEmbeddingModel(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		inputs  []string
		options *embedding.Options
		wantErr bool
	}{
		{
			name:    "single short string",
			inputs:  []string{testEmbeddingInput},
			wantErr: false,
		},
		{
			name:    "single long string",
			inputs:  []string{testEmbeddingInputLong},
			wantErr: false,
		},
		{
			name:    "chinese string",
			inputs:  []string{testEmbeddingInputChinese},
			wantErr: false,
		},
		{
			name: "multiple strings",
			inputs: []string{
				testEmbeddingInput,
				testEmbeddingInputLong,
				testEmbeddingInputChinese,
			},
			wantErr: false,
		},
		{
			name: "batch of similar strings",
			inputs: []string{
				"first test string",
				"second test string",
				"third test string",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := embedding.NewRequest(tt.inputs)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			if tt.options != nil {
				req.Options = tt.options
			}

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			if resp.Metadata == nil {
				t.Error("expected non-nil metadata")
			}

			results := resp.Results
			if len(results) != len(tt.inputs) {
				t.Errorf("expected %d results, got %d", len(tt.inputs), len(results))
			}

			for i, result := range results {
				if result == nil {
					t.Errorf("result %d is nil", i)
					continue
				}

				emb := result.Embedding
				if len(emb) == 0 {
					t.Errorf("result %d has empty embedding", i)
				}

				if result.Metadata == nil {
					t.Errorf("result %d has nil metadata", i)
				} else {
					if result.Metadata.Index != int64(i) {
						t.Errorf("result %d: expected index %d, got %d", i, i, result.Metadata.Index)
					}
				}

				t.Logf("Result %d: embedding dimension = %d", i, len(emb))
			}

			if resp.Metadata.Usage == nil {
				t.Error("expected non-nil usage")
			} else {
				t.Logf("Token usage: prompt=%d", resp.Metadata.Usage.PromptTokens)
			}
		})
	}
}

func TestEmbeddingModel_Call_WithOptions(t *testing.T) {
	model := newTestEmbeddingModel(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		inputs  []string
		options *embedding.Options
		wantErr bool
	}{
		{
			name:   "with custom dimensions",
			inputs: []string{testEmbeddingInput},
			options: func() *embedding.Options {
				opts := assert.Must(embedding.NewOptions(currentConfig.embedModel))
				dims := int64(512)
				opts.Dimensions = &dims
				return opts
			}(),
			wantErr: false,
		},
		{
			name:   "with encoding format",
			inputs: []string{testEmbeddingInput},
			options: func() *embedding.Options {
				opts := assert.Must(embedding.NewOptions(currentConfig.embedModel))
				opts.EncodingFormat = "float"
				return opts
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := embedding.NewRequest(tt.inputs)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.Options = tt.options

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			t.Logf("Embedding dimension: %d", len(result.Embedding))
		})
	}
}

func TestEmbeddingModel_Dimensions(t *testing.T) {
	model := newTestEmbeddingModel(t)

	dims := model.Dimensions()

	t.Logf("Model dimensions: %d", dims)
}

func TestEmbeddingModel_DefaultOptions(t *testing.T) {
	model := newTestEmbeddingModel(t)

	opts := model.DefaultOptions()
	if opts == nil {
		t.Fatal("expected non-nil default options")
	}

	if opts.Model == "" {
		t.Error("expected non-empty model name")
	}

	t.Logf("Default model: %s", opts.Model)
}

func TestEmbeddingModel_Info(t *testing.T) {
	model := newTestEmbeddingModel(t)

	info := model.Info()

	if info.Provider == "" {
		t.Error("expected non-empty provider")
	}

	if info.Provider != Provider {
		t.Errorf("expected provider %q, got %q", Provider, info.Provider)
	}

	t.Logf("Model info: provider=%s", info.Provider)
}

func TestEmbeddingModel_Consistency(t *testing.T) {
	model := newTestEmbeddingModel(t)
	ctx := context.Background()

	input := testEmbeddingInput
	req, err := embedding.NewRequest([]string{input})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp1, err := model.Call(ctx, req)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	resp2, err := model.Call(ctx, req)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	emb1 := resp1.Result().Embedding
	emb2 := resp2.Result().Embedding

	if len(emb1) != len(emb2) {
		t.Fatalf("embedding dimensions differ: %d vs %d", len(emb1), len(emb2))
	}

	var totalDiff float64
	for i := range emb1 {
		diff := emb1[i] - emb2[i]
		if diff < 0 {
			diff = -diff
		}
		totalDiff += diff
	}
	avgDiff := totalDiff / float64(len(emb1))

	t.Logf("Average difference between embeddings: %f", avgDiff)

	if avgDiff > 0.001 {
		t.Logf("Warning: embeddings show significant variation (avg diff: %f)", avgDiff)
	}
}

func TestEmbeddingModel_Similarity(t *testing.T) {
	model := newTestEmbeddingModel(t)
	ctx := context.Background()

	inputs := []string{
		"The cat sits on the mat",
		"A feline rests on a rug",
		"The weather is sunny today",
	}

	req, err := embedding.NewRequest(inputs)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := model.Call(ctx, req)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	results := resp.Results
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	cosineSimilarity := func(a, b []float64) float64 {
		if len(a) != len(b) {
			return 0
		}
		var dotProduct, normA, normB float64
		for i := range a {
			dotProduct += a[i] * b[i]
			normA += a[i] * a[i]
			normB += b[i] * b[i]
		}
		if normA == 0 || normB == 0 {
			return 0
		}
		return dotProduct / (normA * normB)
	}

	sim01 := cosineSimilarity(results[0].Embedding, results[1].Embedding)
	sim02 := cosineSimilarity(results[0].Embedding, results[2].Embedding)

	t.Logf("Similarity between similar sentences: %f", sim01)
	t.Logf("Similarity between dissimilar sentences: %f", sim02)

	if sim01 <= sim02 {
		t.Logf("Warning: similar sentences have lower similarity (%f) than dissimilar ones (%f)", sim01, sim02)
	}
}
