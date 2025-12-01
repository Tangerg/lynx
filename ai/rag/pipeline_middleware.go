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

	mw := &pipelineMiddleware{
		pipeline: pipeline,
	}

	return mw.wrapCallHandler, mw.wrapStreamHandler, nil
}

// runPipeline executes the RAG pipeline on the chat request's last user message
// and returns the augmented query along with retrieved documents.
func (m *pipelineMiddleware) runPipeline(ctx context.Context, req *chat.Request) (*Query, []*document.Document, error) {
	// Build query from the last user message
	query, err := NewQuery(req.UserMessage().Text)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create query: %w", err)
	}

	// Transfer chat request parameters to query context
	for key, value := range req.Params {
		query.Set(key, value)
	}

	// Attach chat history to query context for potential use in retrieval
	query.Set(ChatHistoryKey, req.Messages)

	// Execute the complete RAG pipeline
	augmentedQuery, retrievedDocs, err := m.pipeline.Execute(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("RAG pipeline execution failed: %w", err)
	}

	return augmentedQuery, retrievedDocs, nil
}

// executeCall processes a synchronous chat call through the RAG pipeline.
func (m *pipelineMiddleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	// Execute RAG pipeline to get augmented query and documents
	augmentedQuery, retrievedDocs, err := m.runPipeline(ctx, req)
	if err != nil {
		return nil, err
	}

	// Replace user message with augmented query text
	req.ReplaceOfLastUserMessage(augmentedQuery.Text)

	// Call the next handler with augmented request
	resp, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}

	// Attach retrieved documents to response metadata for downstream access
	resp.Metadata.Set(DocumentContextKey, retrievedDocs)

	return resp, nil
}

// executeStream processes a streaming chat call through the RAG pipeline.
func (m *pipelineMiddleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		// Execute RAG pipeline once before streaming begins
		augmentedQuery, retrievedDocs, err := m.runPipeline(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		// Replace user message with augmented query text
		req.ReplaceOfLastUserMessage(augmentedQuery.Text)

		// Stream responses from the next handler
		for resp, err := range next.Stream(ctx, req) {
			if err != nil {
				yield(resp, err)
				return
			}

			// Attach retrieved documents to each streamed response chunk
			resp.Metadata.Set(DocumentContextKey, retrievedDocs)

			// Yield to consumer; break if consumer signals completion
			if !yield(resp, nil) {
				return
			}
		}
	}
}

// wrapCallHandler returns a CallMiddleware that wraps the next handler.
func (m *pipelineMiddleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler returns a StreamMiddleware that wraps the next handler.
func (m *pipelineMiddleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
