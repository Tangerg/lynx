package rag

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/chat"
)

// Mock call handler
type mockCallHandler struct {
	callFunc func(ctx context.Context, req *chat.Request) (*chat.Response, error)
}

func (m *mockCallHandler) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	if m.callFunc != nil {
		return m.callFunc(ctx, req)
	}
	// Create a valid response
	result, err := chat.NewResult(
		chat.NewAssistantMessage("mock response"),
		&chat.ResultMetadata{
			FinishReason: chat.FinishReasonStop,
		},
	)
	if err != nil {
		return nil, err
	}

	return chat.NewResponse(
		[]*chat.Result{result},
		&chat.ResponseMetadata{
			Model: "mock-model",
		},
	)
}

// Mock stream handler
type mockStreamHandler struct {
	streamFunc func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error]
}

func (m *mockStreamHandler) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	if m.streamFunc != nil {
		return m.streamFunc(ctx, req)
	}
	return func(yield func(*chat.Response, error) bool) {
		result, err := chat.NewResult(
			chat.NewAssistantMessage("mock stream response"),
			&chat.ResultMetadata{
				FinishReason: chat.FinishReasonStop,
			},
		)
		if err != nil {
			yield(nil, err)
			return
		}

		response, err := chat.NewResponse(
			[]*chat.Result{result},
			&chat.ResponseMetadata{
				Model: "mock-model",
			},
		)
		if err != nil {
			yield(nil, err)
			return
		}

		yield(response, nil)
	}
}

func TestNewPipelineMiddleware(t *testing.T) {
	tests := []struct {
		name    string
		config  *PipelineConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "invalid config",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: &PipelineConfig{
				QueryTransformers:  []QueryTransformer{&mockQueryTransformer{}},
				QueryExpander:      &mockQueryExpander{},
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
				DocumentRefiners:   []DocumentRefiner{&mockDocumentRefiner{}},
				QueryAugmenter:     &mockQueryAugmenter{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callMiddleware, streamMiddleware, err := NewPipelineMiddleware(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, callMiddleware)
				assert.Nil(t, streamMiddleware)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, callMiddleware)
				assert.NotNil(t, streamMiddleware)
			}
		})
	}
}

func TestPipelineMiddleware_executeRAG(t *testing.T) {
	tests := []struct {
		name     string
		config   *PipelineConfig
		request  *chat.Request
		wantErr  bool
		validate func(t *testing.T, query *Query, docs []*document.Document)
	}{
		{
			name: "successful execution",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "What is AI?"}),
				},
			},
			wantErr: false,
			validate: func(t *testing.T, query *Query, docs []*document.Document) {
				assert.NotNil(t, query)
				assert.NotEmpty(t, docs)
				// Check chat history is attached
				history, exists := query.Get(ChatHistoryKey)
				assert.True(t, exists)
				assert.NotNil(t, history)
			},
		},
		{
			name: "with request params",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test query"}),
				},
				Params: map[string]any{
					"user_id": "123",
					"source":  "web",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, query *Query, docs []*document.Document) {
				userId, exists := query.Get("user_id")
				assert.True(t, exists)
				assert.Equal(t, "123", userId)
				source, exists := query.Get("source")
				assert.True(t, exists)
				assert.Equal(t, "web", source)
			},
		},
		{
			name: "with chat history",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "What is ML?"}),
					chat.NewAssistantMessage("Machine Learning is..."),
					chat.NewUserMessage(chat.MessageParams{Text: "Tell me more"}),
				},
			},
			wantErr: false,
			validate: func(t *testing.T, query *Query, docs []*document.Document) {
				history, exists := query.Get(ChatHistoryKey)
				assert.True(t, exists)
				messages, ok := history.([]chat.Message)
				assert.True(t, ok)
				assert.Len(t, messages, 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.config)
			require.NoError(t, err)

			middleware := &pipelineMiddleware{
				pipeline: pipeline,
			}

			ctx := context.Background()
			query, docs, err := middleware.executeRAG(ctx, tt.request)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, query, docs)
				}
			}
		})
	}
}

func TestPipelineMiddleware_handleCall(t *testing.T) {
	tests := []struct {
		name     string
		config   *PipelineConfig
		request  *chat.Request
		handler  *mockCallHandler
		wantErr  bool
		validate func(t *testing.T, req *chat.Request, resp *chat.Response)
	}{
		{
			name: "successful call",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "What is AI?"}),
				},
			},
			handler: &mockCallHandler{},
			wantErr: false,
			validate: func(t *testing.T, req *chat.Request, resp *chat.Response) {
				assert.Equal(t, "What is AI?", req.UserMessage().Text)
				// Check documents are attached
				docs, exists := resp.Metadata.Get(DocumentContextKey)
				assert.True(t, exists)
				assert.NotNil(t, docs)
			},
		},
		{
			name: "with augmentation",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
				QueryAugmenter:     &mockQueryAugmenter{},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test"}),
				},
			},
			handler: &mockCallHandler{},
			wantErr: false,
			validate: func(t *testing.T, req *chat.Request, resp *chat.Response) {
				assert.Contains(t, req.UserMessage().Text, "augmented")
			},
		},
		{
			name: "handler returns error",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test"}),
				},
			},
			handler: &mockCallHandler{
				callFunc: func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
					return nil, errors.New("handler error")
				},
			},
			wantErr: true,
		},
		{
			name: "pipeline fails",
			config: &PipelineConfig{
				QueryTransformers: []QueryTransformer{
					&mockQueryTransformer{
						transformFunc: func(ctx context.Context, query *Query) (*Query, error) {
							return nil, errors.New("transform error")
						},
					},
				},
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test"}),
				},
			},
			handler: &mockCallHandler{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.config)
			require.NoError(t, err)

			middleware := &pipelineMiddleware{
				pipeline: pipeline,
			}

			ctx := context.Background()
			response, err := middleware.handleCall(ctx, tt.request, tt.handler)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tt.request, response)
				}
			}
		})
	}
}

func TestPipelineMiddleware_handleStream(t *testing.T) {
	tests := []struct {
		name     string
		config   *PipelineConfig
		request  *chat.Request
		handler  *mockStreamHandler
		wantErr  bool
		validate func(t *testing.T, req *chat.Request, responses []*chat.Response)
	}{
		{
			name: "successful stream",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "What is AI?"}),
				},
			},
			handler: &mockStreamHandler{},
			wantErr: false,
			validate: func(t *testing.T, req *chat.Request, responses []*chat.Response) {
				assert.NotEmpty(t, responses)
				assert.Equal(t, "What is AI?", req.UserMessage().Text)
				// Check documents are attached to each response
				for _, resp := range responses {
					docs, exists := resp.Metadata.Get(DocumentContextKey)
					assert.True(t, exists)
					assert.NotNil(t, docs)
				}
			},
		},
		{
			name: "multiple stream responses",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test"}),
				},
			},
			handler: &mockStreamHandler{
				streamFunc: func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
					return func(yield func(*chat.Response, error) bool) {
						for i := 0; i < 3; i++ {
							result, err := chat.NewResult(
								chat.NewAssistantMessage("chunk"),
								&chat.ResultMetadata{
									FinishReason: chat.FinishReasonStop,
								},
							)
							if err != nil {
								yield(nil, err)
								return
							}

							resp, err := chat.NewResponse(
								[]*chat.Result{result},
								&chat.ResponseMetadata{
									Model: "mock-model",
								},
							)
							if err != nil {
								yield(nil, err)
								return
							}

							if !yield(resp, nil) {
								return
							}
						}
					}
				},
			},
			wantErr: false,
			validate: func(t *testing.T, req *chat.Request, responses []*chat.Response) {
				assert.Len(t, responses, 3)
				for _, resp := range responses {
					docs, exists := resp.Metadata.Get(DocumentContextKey)
					assert.True(t, exists)
					assert.NotNil(t, docs)
				}
			},
		},
		{
			name: "stream with error",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test"}),
				},
			},
			handler: &mockStreamHandler{
				streamFunc: func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
					return func(yield func(*chat.Response, error) bool) {
						yield(nil, errors.New("stream error"))
					}
				},
			},
			wantErr: true,
		},
		{
			name: "pipeline fails before streaming",
			config: &PipelineConfig{
				DocumentRetrievers: []DocumentRetriever{
					&mockDocumentRetriever{
						retrieveFunc: func(ctx context.Context, query *Query) ([]*document.Document, error) {
							return nil, errors.New("retrieve error")
						},
					},
				},
			},
			request: &chat.Request{
				Messages: []chat.Message{
					chat.NewUserMessage(chat.MessageParams{Text: "test"}),
				},
			},
			handler: &mockStreamHandler{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.config)
			require.NoError(t, err)

			middleware := &pipelineMiddleware{
				pipeline: pipeline,
			}

			ctx := context.Background()
			stream := middleware.handleStream(ctx, tt.request, tt.handler)

			var responses []*chat.Response
			var streamErr error

			for response, err := range stream {
				if err != nil {
					streamErr = err
					break
				}
				responses = append(responses, response)
			}

			if tt.wantErr {
				assert.Error(t, streamErr)
			} else {
				assert.NoError(t, streamErr)
				if tt.validate != nil {
					tt.validate(t, tt.request, responses)
				}
			}
		})
	}
}

func TestPipelineMiddleware_callMiddleware(t *testing.T) {
	config := &PipelineConfig{
		DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
		QueryAugmenter:     &mockQueryAugmenter{},
	}

	callMiddleware, _, err := NewPipelineMiddleware(config)
	require.NoError(t, err)

	handler := &mockCallHandler{}
	wrappedHandler := callMiddleware(handler)

	ctx := context.Background()
	request := &chat.Request{
		Messages: []chat.Message{
			chat.NewUserMessage(chat.MessageParams{Text: "test query"}),
		},
	}

	response, err := wrappedHandler.Call(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, response)

	// Verify request was augmented
	assert.Contains(t, request.UserMessage().Text, "augmented")

	// Verify documents are attached
	docs, exists := response.Metadata.Get(DocumentContextKey)
	assert.True(t, exists)
	assert.NotNil(t, docs)
}

func TestPipelineMiddleware_streamMiddleware(t *testing.T) {
	config := &PipelineConfig{
		DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
		QueryAugmenter:     &mockQueryAugmenter{},
	}

	_, streamMiddleware, err := NewPipelineMiddleware(config)
	require.NoError(t, err)

	handler := &mockStreamHandler{}
	wrappedHandler := streamMiddleware(handler)

	ctx := context.Background()
	request := &chat.Request{
		Messages: []chat.Message{
			chat.NewUserMessage(chat.MessageParams{Text: "test query"}),
		},
	}

	stream := wrappedHandler.Stream(ctx, request)

	var responses []*chat.Response
	for response, err := range stream {
		require.NoError(t, err)
		responses = append(responses, response)
	}

	assert.NotEmpty(t, responses)

	// Verify request was augmented
	assert.Contains(t, request.UserMessage().Text, "augmented")

	// Verify documents are attached to all responses
	for _, response := range responses {
		docs, exists := response.Metadata.Get(DocumentContextKey)
		assert.True(t, exists)
		assert.NotNil(t, docs)
	}
}

func TestPipelineMiddleware_Integration(t *testing.T) {
	config := &PipelineConfig{
		QueryTransformers:  []QueryTransformer{&mockQueryTransformer{}},
		QueryExpander:      &mockQueryExpander{},
		DocumentRetrievers: []DocumentRetriever{&mockDocumentRetriever{}},
		DocumentRefiners:   []DocumentRefiner{&mockDocumentRefiner{}},
		QueryAugmenter:     &mockQueryAugmenter{},
	}

	callMiddleware, streamMiddleware, err := NewPipelineMiddleware(config)
	require.NoError(t, err)

	t.Run("call integration", func(t *testing.T) {
		handler := &mockCallHandler{}
		wrappedHandler := callMiddleware(handler)

		ctx := context.Background()
		request := &chat.Request{
			Messages: []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "test"}),
			},
			Params: map[string]any{
				"user_id": "123",
			},
		}

		response, err := wrappedHandler.Call(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, response)

		// Verify full pipeline was executed
		assert.NotContains(t, request.UserMessage().Text, "transformed")
		assert.Contains(t, request.UserMessage().Text, "augmented")

		docs, exists := response.Metadata.Get(DocumentContextKey)
		assert.True(t, exists)
		assert.NotNil(t, docs)
	})

	t.Run("stream integration", func(t *testing.T) {
		handler := &mockStreamHandler{}
		wrappedHandler := streamMiddleware(handler)

		ctx := context.Background()
		request := &chat.Request{
			Messages: []chat.Message{
				chat.NewUserMessage(chat.MessageParams{Text: "test"}),
			},
		}

		stream := wrappedHandler.Stream(ctx, request)

		var responses []*chat.Response
		for response, err := range stream {
			require.NoError(t, err)
			responses = append(responses, response)
		}

		assert.NotEmpty(t, responses)

		// Verify full pipeline was executed
		assert.NotContains(t, request.UserMessage().Text, "transformed")
		assert.Contains(t, request.UserMessage().Text, "augmented")

		for _, response := range responses {
			docs, exists := response.Metadata.Get(DocumentContextKey)
			assert.True(t, exists)
			assert.NotNil(t, docs)
		}
	})
}

func TestDocumentContextKey_Constant(t *testing.T) {
	assert.Equal(t, "lynx:ai:rag:document_context", DocumentContextKey)
}

func TestChatHistoryKey_Constant(t *testing.T) {
	assert.Equal(t, "lynx:ai:rag:chat_history", ChatHistoryKey)
}
