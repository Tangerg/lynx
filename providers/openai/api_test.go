package openai

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
)

type testConfig struct {
	baseURL    string
	chatModel  string
	embedModel string
	imageModel string
}

var (
	configs = map[string]testConfig{
		"siliconflow": {
			baseURL:    "https://api.siliconflow.cn/v1",
			chatModel:  "Qwen/Qwen2.5-7B-Instruct",
			embedModel: "BAAI/bge-m3",
			imageModel: "Qwen/Qwen-Image",
		},
		"moonshot": {
			baseURL:   "https://api.moonshot.cn/v1",
			chatModel: "moonshot-v1-8k-vision-preview",
		},
	}

	currentConfig = configs["siliconflow"]
)

func getAPIKey(t *testing.T) model.ApiKey {
	t.Helper()
	apiKey := os.Getenv("apiKey")
	if apiKey == "" {
		t.Skip("apiKey environment variable not set")
	}
	return model.NewApiKey(apiKey)
}

func newTestAPI(t *testing.T, opts ...option.RequestOption) *Api {
	t.Helper()

	defaultOpts := []option.RequestOption{
		option.WithBaseURL(currentConfig.baseURL),
	}
	defaultOpts = append(defaultOpts, opts...)

	api, err := NewApi(&ApiConfig{
		ApiKey:         getAPIKey(t),
		RequestOptions: defaultOpts,
	})
	if err != nil {
		t.Fatalf("failed to create API instance: %v", err)
	}

	return api
}

func TestNewApi(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  model.ApiKey
		opts    []option.RequestOption
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid api key",
			apiKey:  getAPIKey(t),
			opts:    []option.RequestOption{option.WithBaseURL(currentConfig.baseURL)},
			wantErr: false,
		},
		{
			name:    "nil api key",
			apiKey:  nil,
			wantErr: true,
			errMsg:  "apiKey is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, err := NewApi(&ApiConfig{
				ApiKey:         tt.apiKey,
				RequestOptions: tt.opts,
			})

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

			if api == nil {
				t.Error("expected non-nil API instance")
			}
		})
	}
}

func TestApi_ChatCompletion(t *testing.T) {
	api := newTestAPI(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		params  *openai.ChatCompletionNewParams
		wantErr bool
	}{
		{
			name: "simple message",
			params: &openai.ChatCompletionNewParams{
				Model: currentConfig.chatModel,
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Hi! Say hello."),
				},
			},
			wantErr: false,
		},
		{
			name: "multi-turn conversation",
			params: &openai.ChatCompletionNewParams{
				Model: currentConfig.chatModel,
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.SystemMessage("You are a helpful assistant."),
					openai.UserMessage("What is 2+2?"),
					openai.AssistantMessage("2+2 equals 4."),
					openai.UserMessage("What about 3+3?"),
				},
			},
			wantErr: false,
		},
		{
			name:    "nil params",
			params:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completion, err := api.ChatCompletion(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if completion == nil {
				t.Fatal("expected non-nil completion")
			}

			if len(completion.Choices) == 0 {
				t.Fatal("expected at least one choice")
			}

			content := completion.Choices[0].Message.Content
			if content == "" {
				t.Error("expected non-empty content")
			}

			t.Logf("Response: %s", content)
		})
	}
}

func TestApi_ChatCompletionStream(t *testing.T) {
	api := newTestAPI(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		params  *openai.ChatCompletionNewParams
		wantErr bool
	}{
		{
			name: "simple streaming",
			params: &openai.ChatCompletionNewParams{
				Model: currentConfig.chatModel,
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Count from 1 to 5."),
				},
			},
			wantErr: false,
		},
		{
			name:    "nil params",
			params:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := api.ChatCompletionStream(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if stream == nil {
				t.Fatal("expected non-nil stream")
			}

			accumulator := openai.ChatCompletionAccumulator{}
			chunkCount := 0

			for stream.Next() {
				chunk := stream.Current()
				accumulator.AddChunk(chunk)
				chunkCount++

				if len(chunk.Choices) > 0 {
					delta := chunk.Choices[0].Delta.Content
					t.Logf("Chunk %d: %s", chunkCount, delta)
				}
			}

			if err := stream.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}

			if chunkCount == 0 {
				t.Error("expected at least one chunk")
			}

			if len(accumulator.Choices) > 0 {
				finalContent := accumulator.Choices[0].Message.Content
				if finalContent == "" {
					t.Error("expected non-empty final content")
				}
				t.Logf("Final content: %s", finalContent)
			}
		})
	}
}

func TestApi_Embeddings(t *testing.T) {
	if currentConfig.embedModel == "" {
		t.Skip("embed model not configured")
	}

	api := newTestAPI(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		params  *openai.EmbeddingNewParams
		wantErr bool
	}{
		{
			name: "single string",
			params: &openai.EmbeddingNewParams{
				Model: currentConfig.embedModel,
				Input: openai.EmbeddingNewParamsInputUnion{
					OfString: openai.String("test embedding string"),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple strings",
			params: &openai.EmbeddingNewParams{
				Model: currentConfig.embedModel,
				Input: openai.EmbeddingNewParamsInputUnion{
					OfArrayOfStrings: []string{
						"first test string",
						"second test string",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil params",
			params:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embeddings, err := api.Embedding(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if embeddings == nil {
				t.Fatal("expected non-nil embeddings")
			}

			if len(embeddings.Data) == 0 {
				t.Fatal("expected at least one embedding")
			}

			for i, data := range embeddings.Data {
				if len(data.Embedding) == 0 {
					t.Errorf("embedding %d is empty", i)
				}
				t.Logf("Embedding %d dimension: %d", i, len(data.Embedding))
			}
		})
	}
}

func TestApi_Images(t *testing.T) {
	if currentConfig.imageModel == "" {
		t.Skip("image model not configured")
	}

	api := newTestAPI(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		params  *openai.ImageGenerateParams
		wantErr bool
	}{
		{
			name: "simple generation",
			params: &openai.ImageGenerateParams{
				Model:  currentConfig.imageModel,
				Prompt: "a beautiful sunset over the ocean",
			},
			wantErr: false,
		},
		{
			name: "detailed prompt",
			params: &openai.ImageGenerateParams{
				Model:  currentConfig.imageModel,
				Prompt: "an island near sea, with seagulls, moon shining over the sea, light house, boats in the background",
			},
			wantErr: false,
		},
		{
			name:    "nil params",
			params:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := api.Image(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if response == nil {
				t.Fatal("expected non-nil response")
			}

			if len(response.Data) == 0 {
				t.Fatal("expected at least one image")
			}

			for i, img := range response.Data {
				hasURL := img.URL != ""
				hasB64 := img.B64JSON != ""

				if !hasURL && !hasB64 {
					t.Errorf("image %d has neither URL nor B64JSON", i)
				}

				if hasURL {
					t.Logf("Image %d URL: %s", i, img.URL)
				}
				if hasB64 {
					t.Logf("Image %d has B64JSON (length: %d)", i, len(img.B64JSON))
				}
			}
		})
	}
}
