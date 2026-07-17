package arch

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/moderation"
	"github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func representativeWireContracts(t *testing.T) map[string]any {
	t.Helper()

	protocolMetadata := mustMetadata(t, map[string]any{
		"count":  2,
		"label":  "fixture",
		"nested": map[string]any{"enabled": true},
	})
	inlineMedia, err := media.NewBytes("image/png", []byte("lynx"))
	if err != nil {
		t.Fatal(err)
	}
	inlineMedia.ID = "media-1"
	inlineMedia.Name = "lynx.png"
	inlineMedia.Metadata = protocolMetadata.Clone()
	uriMedia, err := media.NewURI("audio/mpeg", "https://example.com/lynx.mp3")
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
	chatRequest.Options = chat.Options{
		Model:            "chat-model",
		FrequencyPenalty: new(0.1),
		MaxTokens:        new(int64(512)),
		PresencePenalty:  new(0.2),
		Stop:             []string{"END"},
		Temperature:      new(0.3),
		TopK:             new(int64(40)),
		TopP:             new(0.9),
	}
	mustSetChatExtension(t, chatRequest.SetExtension("provider/request", map[string]any{"mode": "strict"}))

	assistant := chat.NewAssistantMessage(chat.NewTextPart("A lynx."))
	chatResponse, err := chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &assistant,
		FinishReason: chat.FinishReasonStop,
		Extensions:   mustMetadata(t, map[string]any{"provider/logprob": -0.25}),
	})
	if err != nil {
		t.Fatal(err)
	}
	chatResponse.ID = "response-1"
	chatResponse.Model = "chat-model"
	chatResponse.Usage = chat.Usage{
		InputTokens:           10,
		OutputTokens:          5,
		ReasoningTokens:       new(int64(2)),
		CacheReadInputTokens:  new(int64(3)),
		CacheWriteInputTokens: new(int64(4)),
	}
	mustSetChatExtension(t, chatResponse.SetExtension("provider/response", "fixture"))

	doc := &document.Document{
		ID:       "doc-1",
		Text:     "A lynx document.",
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
		Texts: []string{"lynx", "wild cat"},
		Options: embedding.Options{
			Model:      "embedding-model",
			Dimensions: new(int64(3)),
			Extensions: mustMetadata(t, map[string]any{"fixture/options": map[string]any{"user": "u-1"}}),
		},
	}
	embeddingResponse := &embedding.Response{
		Results: []*embedding.Result{{
			Embedding: []float64{0.1, 0.2, 0.3},
			Metadata: &embedding.ResultMetadata{
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
		Prompt: "A lynx in snow",
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
		Results: []*image.Result{{
			Media:    generatedMedia,
			Metadata: &image.ResultMetadata{Extra: mustMetadata(t, map[string]any{"revised_prompt": "A detailed lynx"})},
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
		Results: []*moderation.Result{{
			Categories: representativeCategories(),
			Metadata:   &moderation.ResultMetadata{Extra: mustMetadata(t, map[string]any{"input_index": 1})},
		}},
		Metadata: &moderation.ResponseMetadata{
			ID:      "moderation-1",
			Model:   "moderation-model",
			Created: 1700000002,
			Extra:   mustMetadata(t, map[string]any{"region": "local"}),
		},
	}

	speechRequest := &speech.Request{
		Text: "Hello from Lynx.",
		Options: speech.Options{
			Model:        "speech-model",
			Voice:        "alloy",
			OutputFormat: "mp3",
			Speed:        1.25,
			Extensions:   mustMetadata(t, map[string]any{"fixture/options": map[string]any{"style": "calm"}}),
		},
	}
	speechResponse := &speech.Response{
		Result: &speech.Result{
			Audio:    []byte("audio"),
			Metadata: &speech.ResultMetadata{Extra: mustMetadata(t, map[string]any{"duration_ms": 250})},
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
		Result: &transcription.Result{
			Text:     "A lynx.",
			Metadata: &transcription.ResultMetadata{Extra: mustMetadata(t, map[string]any{"duration": 1.5})},
		},
		Metadata: &transcription.ResponseMetadata{
			Model:   "transcription-model",
			Created: 1700000004,
			Extra:   mustMetadata(t, map[string]any{"language": "en"}),
		},
	}

	return map[string]any{
		"chat_request":               chatRequest,
		"chat_response":              chatResponse,
		"document":                   doc,
		"document_empty_metadata":    emptyMetadataDoc,
		"embedding_request":          embeddingRequest,
		"embedding_response":         embeddingResponse,
		"image_request":              imageRequest,
		"image_response":             imageResponse,
		"media":                      []*media.Media{inlineMedia, uriMedia, referenceMedia},
		"metadata":                   protocolMetadata,
		"moderation_request":         moderationRequest,
		"moderation_response":        moderationResponse,
		"speech_request":             speechRequest,
		"speech_response":            speechResponse,
		"transcription_request":      transcriptionRequest,
		"transcription_response":     transcriptionResponse,
		"vectorstore_match":          vectorstore.Match{Document: doc, Score: 0.95},
		"vectorstore_search_request": vectorstore.SearchRequest{Query: "lynx", TopK: 10, MinScore: 0.75, Filter: filter.EQ("kind", "animal")},
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
	result, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustSetChatExtension(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
