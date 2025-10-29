package rag

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/chat"
)

const (
	// DocumentContextKey is the metadata key for storing retrieved documents in chat responses.
	DocumentContextKey = "lynx:ai:rag:document_context"

	// ChatHistoryKey is the metadata key for storing chat history in the query context.
	ChatHistoryKey = "lynx:ai:rag:chat_history"
)

// pipelineMiddleware wraps a RAG pipeline as middleware for chat model calls.
// It intercepts chat requests, executes the RAG pipeline, and augments both
// the request and response with retrieved documents.
type pipelineMiddleware struct {
	pipeline *Pipeline
}

// NewPipelineMiddleware creates new chat middleware from a RAG pipeline configuration.
// It returns both call and stream middleware, or an error if the pipeline creation fails.
func NewPipelineMiddleware(config *PipelineConfig) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	pipeline, err := NewPipeline(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create pipeline middleware: %w", err)
	}

	middleware := &pipelineMiddleware{
		pipeline: pipeline,
	}

	return middleware.callMiddleware, middleware.streamMiddleware, nil
}

// executeRAG runs the RAG pipeline on the chat request's last user message.
func (m *pipelineMiddleware) executeRAG(ctx context.Context, req *chat.Request) (*Query, []*document.Document, error) {
	// Build query from the last user message
	query, err := NewQuery(req.UserMessage().Text)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create query: %w", err)
	}

	// Attach chat request params
	for key, val := range req.Params {
		query.Set(key, val)
	}

	// Attach chat history to query context for potential use in retrieval
	query.Set(ChatHistoryKey, req.Messages)

	// Execute the complete RAG pipeline
	augmented, docs, err := m.pipeline.Execute(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("RAG pipeline execution failed: %w", err)
	}

	return augmented, docs, nil
}

// handleCall processes a synchronous chat call through the RAG pipeline.
func (m *pipelineMiddleware) handleCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	// Execute RAG pipeline
	augmented, docs, err := m.executeRAG(ctx, req)
	if err != nil {
		return nil, err
	}

	// Replace user message with augmented query
	req.ReplaceOfLastUserMessage(augmented.Text)

	// Call the next handler with augmented request
	response, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}

	// Attach retrieved documents to response metadata
	response.Metadata.Set(DocumentContextKey, docs)

	return response, nil
}

// handleStream processes a streaming chat call through the RAG pipeline.
func (m *pipelineMiddleware) handleStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		// Execute RAG pipeline once before streaming
		augmented, docs, err := m.executeRAG(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		// Replace user message with augmented query
		req.ReplaceOfLastUserMessage(augmented.Text)

		// Stream responses from the next handler
		for response, err := range next.Stream(ctx, req) {
			if err != nil {
				yield(response, err)
				return
			}

			// Attach retrieved documents to each streamed response
			response.Metadata.Set(DocumentContextKey, docs)

			// Yield to consumer
			if !yield(response, nil) {
				return
			}
		}
	}
}

// callMiddleware returns a CallMiddleware that wraps the next handler.
func (m *pipelineMiddleware) callMiddleware(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.handleCall(ctx, req, next)
	})
}

// streamMiddleware returns a StreamMiddleware that wraps the next handler.
func (m *pipelineMiddleware) streamMiddleware(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.handleStream(ctx, req, next)
	})
}
