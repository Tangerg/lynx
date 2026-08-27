package arch

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/image"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/core/moderation"
	"github.com/Tangerg/scope/core/speech"
	"github.com/Tangerg/scope/core/transcription"
	"github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func representativeWireContracts(t *testing.T) map[string]any {
	t.Helper()

	protocolMetadata := mustMetadata(t, map[string]any{
		"count":  2,
		"label":  "fixture",
		"nested": map[string]any{"enabled": true},
	})
	inlineMedia, err := media.NewBytes("image/png", []byte("scope"))
	if err != nil {
		t.Fatal(err)
	}
	inlineMedia.ID = "media-1"
	inlineMedia.Name = "scope.png"
	inlineMedia.Metadata = protocolMetadata.Clone()
	uriMedia, err := media.NewURI("audio/mpeg", "https://example.com/scope.mp3")
	if err != nil {
		t.Fatal(err)
	}
	referenceMedia, err := media.NewReference("video/mp4", "provider-file-1")
	if err != nil {
		t.Fatal(err)
	}
	generatedMedia, err := media.NewURI("image/png", "https://example.com/generated.png")
	if err != nil {
		t.Fatal(err)
	}

	chatRequest, err := chat.NewRequest(
		chat.NewSystemMessage("Answer precisely."),
		chat.NewUserMessage(chat.NewTextPart("Describe the media."), chat.NewMediaPart(inlineMedia)),
		chat.NewAssistantMessage(
			chat.NewReasoningPart("Inspect it.", []byte("signature")),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "inspect", Arguments: `{"detail":"high"}`}),
		),
		chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "inspect", Result: "failed", IsError: true}),
	)
	if err != nil {
		t.Fatal(err)
	}
	chatRequest.Tools = []chat.ToolDefinition{{
		Name:        "inspect",
		Description: "Inspect media",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	chatOutputFormat, err := chat.NewJSONSchemaOutputFormat(
		"answer",
		json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	chatOutputFormat.Description = "Structured answer"
	chatRequest.Options = chat.Options{
		Model:            "chat-model",
		OutputFormat:     &chatOutputFormat,
		FrequencyPenalty: new(0.1),
		MaxTokens:        new(int64(512)),
		PresencePenalty:  new(0.2),
		Stop:             []string{"END"},
		Temperature:      new(0.3),
		TopK:             new(int64(40)),
		TopP:             new(0.9),
	}
	mustSetChatExtension(t, chatRequest.Options.SetExtension("provider/request", map[string]any{"mode": "strict"}))

	assistant := chat.NewAssistantMessage(chat.NewTextPart("A scope."))
	chatResponse, err := chat.NewResponse(&chat.Output{
		Message:      &assistant,
		FinishReason: chat.FinishReasonStop,
		Metadata: &chat.OutputMetadata{
			Extra: mustMetadata(t, map[string]any{"provider/logprob": -0.25}),
		},
	}, &chat.ResponseMetadata{
		ID:    "response-1",
		Model: "chat-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	chatResponse.Metadata.Usage = chat.Usage{
		InputTokens:           10,
		OutputTokens:          5,
		ReasoningTokens:       new(int64(2)),
		CacheReadInputTokens:  new(int64(3)),
		CacheWriteInputTokens: new(int64(4)),
	}
	mustSetChatExtension(t, chatResponse.Metadata.Set("provider/response", "fixture"))

	doc := &document.Document{
		ID:       "doc-1",
		Text:     "A scope document.",
		Media:    inlineMedia,
		Metadata: protocolMetadata.Clone(),
	}
	// emptyMetadataDoc pins the metadata.Map omitzero contract: an explicitly
	// empty (non-nil) Metadata map must serialize with no "metadata" key, not a
	// bare "{}". document.NewDocument seeds exactly such an empty map.
	emptyMetadataDoc, err := document.NewDocument("no metadata", nil)
	if err != nil {
		t.Fatal(err)
	}
	embeddingRequest := &embedding.Request{
		Texts: []string{"scope", "wild cat"},
		Options: embedding.Options{
			Model:      "embedding-model",
			Dimensions: new(int64(3)),
			Extensions: mustMetadata(t, map[string]any{"fixture/options": map[string]any{"user": "u-1"}}),
		},
	}
	embeddingResponse := &embedding.Response{
		Outputs: []*embedding.Output{{
			Embedding: []float64{0.1, 0.2, 0.3},
			Metadata: &embedding.OutputMetadata{
				Extra: mustMetadata(t, map[string]any{"source": "fixture"}),
			},
		}},
		Metadata: &embedding.ResponseMetadata{
			Model:   "embedding-model",
			Usage:   &embedding.Usage{InputTokens: 100},
			Created: 1700000000,
			Extra:   mustMetadata(t, map[string]any{"region": "local"}),
		},
	}

	imageRequest := &image.Request{
		Prompt: "A scope in snow",
		Options: image.Options{
			Model:          "image-model",
			NegativePrompt: "text",
			Width:          new(int64(1024)),
			Height:         new(int64(768)),
			Seed:           new(int64(42)),
			OutputFormat:   "image/png",
			Extensions:     mustMetadata(t, map[string]any{"fixture/options": map[string]any{"background": "transparent"}}),
		},
	}
	imageResponse := &image.Response{
		Outputs: []*image.Output{{
			Media:    generatedMedia,
			Metadata: &image.OutputMetadata{Extra: mustMetadata(t, map[string]any{"revised_prompt": "A detailed scope"})},
		}},
		Metadata: &image.ResponseMetadata{
			Created: 1700000001,
			Extra:   mustMetadata(t, map[string]any{"model": "image-model"}),
		},
	}

	moderationRequest := &moderation.Request{
		Texts: []string{"safe text", "unsafe text"},
		Options: moderation.Options{
			Model:      "moderation-model",
			Extensions: mustMetadata(t, map[string]any{"fixture/options": map[string]any{"policy": "strict"}}),
		},
	}
	moderationResponse := &moderation.Response{
		Outputs: []*moderation.Output{{
			Categories: representativeCategories(),
			Metadata:   &moderation.OutputMetadata{Extra: mustMetadata(t, map[string]any{"input_index": 1})},
		}},
		Metadata: &moderation.ResponseMetadata{
			ID:      "moderation-1",
			Model:   "moderation-model",
			Created: 1700000002,
			Extra:   mustMetadata(t, map[string]any{"region": "local"}),
		},
	}

	speechRequest := &speech.Request{
		Text: "Hello from Scope.",
		Options: speech.Options{
			Model:        "speech-model",
			Voice:        "alloy",
			OutputFormat: "mp3",
			Speed:        1.25,
			Extensions:   mustMetadata(t, map[string]any{"fixture/options": map[string]any{"style": "calm"}}),
		},
	}
	speechResponse := &speech.Response{
		Output: &speech.Output{
			Audio:    []byte("audio"),
			Metadata: &speech.OutputMetadata{Extra: mustMetadata(t, map[string]any{"duration_ms": 250})},
		},
		Metadata: &speech.ResponseMetadata{
			Model:   "speech-model",
			Created: 1700000003,
			Extra:   mustMetadata(t, map[string]any{"format": "mp3"}),
		},
	}

	transcriptionRequest := &transcription.Request{
		Audio: uriMedia,
		Options: transcription.Options{
			Model:      "transcription-model",
			Language:   "en",
			Extensions: mustMetadata(t, map[string]any{"fixture/options": map[string]any{"diarize": true}}),
		},
	}
	transcriptionResponse := &transcription.Response{
		Output: &transcription.Output{
			Text:     "A scope.",
			Metadata: &transcription.OutputMetadata{Extra: mustMetadata(t, map[string]any{"duration": 1.5})},
		},
		Metadata: &transcription.ResponseMetadata{
			Model:   "transcription-model",
			Created: 1700000004,
			Extra:   mustMetadata(t, map[string]any{"language": "en"}),
		},
	}

	return map[string]any{
		"chat_request":              chatRequest,
		"chat_response":             chatResponse,
		"document":                  doc,
		"document_empty_metadata":   emptyMetadataDoc,
		"embedding_request":         embeddingRequest,
		"embedding_response":        embeddingResponse,
		"image_request":             imageRequest,
		"image_response":            imageResponse,
		"media":                     []*media.Media{inlineMedia, uriMedia, referenceMedia},
		"metadata":                  protocolMetadata,
		"moderation_request":        moderationRequest,
		"moderation_response":       moderationResponse,
		"speech_request":            speechRequest,
		"speech_response":           speechResponse,
		"transcription_request":     transcriptionRequest,
		"transcription_response":    transcriptionResponse,
		"vectorstore_index_request": &vectorstore.IndexRequest{Documents: []*document.Document{doc}},
		"vectorstore_search_request": vectorstore.SearchRequest{
			Query: "scope",
			Options: vectorstore.SearchOptions{
				TopK: 10, MinScore: 0.75, Filter: filter.EQ("kind", "animal"),
			},
		},
		"vectorstore_search_response": &vectorstore.SearchResponse{
			Results: []*vectorstore.SearchResult{{Document: doc, Score: 0.95}},
		},
	}
}

func representativeCategories() moderation.Categories {
	return moderation.Categories{
		"sexual":                         {Flagged: true, Score: 0.01},
		"hate":                           {Flagged: true, Score: 0.02},
		"harassment":                     {Flagged: true, Score: 0.03},
		"self_harm":                      {Flagged: true, Score: 0.04},
		"sexual_minors":                  {Flagged: true, Score: 0.05},
		"hate_threatening":               {Flagged: true, Score: 0.06},
		"violence_graphic":               {Flagged: true, Score: 0.07},
		"self_harm_intent":               {Flagged: true, Score: 0.08},
		"self_harm_instructions":         {Flagged: true, Score: 0.09},
		"harassment_threatening":         {Flagged: true, Score: 0.10},
		"violence":                       {Flagged: true, Score: 0.11},
		"dangerous_and_criminal_content": {Flagged: true, Score: 0.12},
		"health":                         {Flagged: true, Score: 0.13},
		"financial":                      {Flagged: true, Score: 0.14},
		"law":                            {Flagged: true, Score: 0.15},
		"pii":                            {Flagged: true, Score: 0.16},
		"illicit":                        {Flagged: true, Score: 0.17},
		"illicit_violent":                {Flagged: true, Score: 0.18},
	}
}

func mustMetadata(t *testing.T, values map[string]any) metadata.Map {
	t.Helper()
	output, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func mustSetChatExtension(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
