package arch

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/model"
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
		FrequencyPenalty: pointer(0.1),
		MaxTokens:        pointer(int64(512)),
		PresencePenalty:  pointer(0.2),
		Stop:             []string{"END"},
		Temperature:      pointer(0.3),
		TopK:             pointer(int64(40)),
		TopP:             pointer(0.9),
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
		ReasoningTokens:       pointer(int64(2)),
		CacheReadInputTokens:  pointer(int64(3)),
		CacheWriteInputTokens: pointer(int64(4)),
	}
	mustSetChatExtension(t, chatResponse.SetExtension("provider/response", "fixture"))

	doc := &document.Document{
		ID:       "doc-1",
		Text:     "A lynx document.",
		Media:    inlineMedia,
		Metadata: protocolMetadata.Clone(),
	}
	usage := model.Usage{
		PromptTokens:          100,
		CompletionTokens:      20,
		ReasoningTokens:       pointer(int64(5)),
		CacheReadInputTokens:  pointer(int64(10)),
		CacheWriteInputTokens: pointer(int64(15)),
		OriginalUsage:         map[string]any{"provider_total": 120},
	}
	rateLimit := model.RateLimit{
		RequestsLimit:     100,
		RequestsRemaining: 99,
		RequestsReset:     30 * time.Second,
		TokensLimit:       10000,
		TokensRemaining:   9880,
		TokensReset:       time.Minute,
	}

	embeddingRequest := &embedding.Request{
		Texts: []string{"lynx", "wild cat"},
		Options: &embedding.Options{
			Model:          "embedding-model",
			EncodingFormat: embedding.EncodingFormatFloat,
			Dimensions:     pointer(int64(3)),
			Extra:          map[string]any{"user": "u-1"},
		},
		Params: map[string]any{"trace_id": "trace-1"},
	}
	embeddingResponse := &embedding.Response{
		Results: []*embedding.Result{{
			Embedding: []float64{0.1, 0.2, 0.3},
			Metadata: &embedding.ResultMetadata{
				Index:        0,
				ModalityType: embedding.Text,
				MIMEType:     "text/plain",
				Extra:        map[string]any{"source": "fixture"},
			},
		}},
		Metadata: &embedding.ResponseMetadata{
			Model:     "embedding-model",
			Usage:     &usage,
			RateLimit: &rateLimit,
			Created:   1700000000,
			Extra:     map[string]any{"region": "local"},
		},
	}

	imageRequest := &image.Request{
		Prompt: "A lynx in snow",
		Options: &image.Options{
			Model:          "image-model",
			NegativePrompt: "text",
			Width:          pointer(int64(1024)),
			Height:         pointer(int64(768)),
			Style:          "natural",
			Quality:        "high",
			Seed:           pointer(int64(42)),
			OutputFormat:   "image/png",
			ResponseFormat: image.ResponseFormatB64JSON,
			Extra:          map[string]any{"background": "transparent"},
		},
		Params: map[string]any{"trace_id": "trace-2"},
	}
	imageResponse := &image.Response{
		Result: &image.Result{
			Image:    &image.Image{URL: "https://example.com/generated.png", B64JSON: "bHlueA=="},
			Metadata: &image.ResultMetadata{Extra: map[string]any{"revised_prompt": "A detailed lynx"}},
		},
		Metadata: &image.ResponseMetadata{
			Created: 1700000001,
			Extra:   map[string]any{"model": "image-model"},
		},
	}

	moderationRequest := &moderation.Request{
		Texts: []string{"safe text", "unsafe text"},
		Options: &moderation.Options{
			Model: "moderation-model",
			Extra: map[string]any{"policy": "strict"},
		},
		Params: map[string]any{"trace_id": "trace-3"},
	}
	moderationResponse := &moderation.Response{
		Results: []*moderation.Result{{
			Categories: representativeCategories(),
			Metadata:   &moderation.ResultMetadata{Extra: map[string]any{"input_index": 1}},
		}},
		Metadata: &moderation.ResponseMetadata{
			ID:      "moderation-1",
			Model:   "moderation-model",
			Created: 1700000002,
			Extra:   map[string]any{"region": "local"},
		},
	}

	speechRequest := &speech.Request{
		Text: "Hello from Lynx.",
		Options: &speech.Options{
			Model:          "speech-model",
			Voice:          "alloy",
			ResponseFormat: "mp3",
			Speed:          1.25,
			Extra:          map[string]any{"style": "calm"},
		},
		Params: map[string]any{"trace_id": "trace-4"},
	}
	speechResponse := &speech.Response{
		Result: &speech.Result{
			Speech:   []byte("audio"),
			Metadata: &speech.ResultMetadata{Extra: map[string]any{"duration_ms": 250}},
		},
		Metadata: &speech.ResponseMetadata{
			Model:   "speech-model",
			Created: 1700000003,
			Extra:   map[string]any{"format": "mp3"},
		},
	}

	transcriptionRequest := &transcription.Request{
		Audio: uriMedia,
		Options: &transcription.Options{
			Model:                "transcription-model",
			Language:             "en",
			Prompt:               "Lynx vocabulary",
			Temperature:          pointer(0.1),
			ResponseFormat:       "verbose_json",
			TimestampGranularity: []string{"word", "segment"},
			Extra:                map[string]any{"diarize": true},
		},
		Params: map[string]any{"trace_id": "trace-5"},
	}
	transcriptionResponse := &transcription.Response{
		Result: &transcription.Result{
			Text:     "A lynx.",
			Metadata: &transcription.ResultMetadata{Extra: map[string]any{"duration": 1.5}},
		},
		Metadata: &transcription.ResponseMetadata{
			Model:   "transcription-model",
			Created: 1700000004,
			Extra:   map[string]any{"language": "en"},
		},
	}

	return map[string]any{
		"chat_request":               chatRequest,
		"chat_response":              chatResponse,
		"document":                   doc,
		"embedding_request":          embeddingRequest,
		"embedding_response":         embeddingResponse,
		"image_request":              imageRequest,
		"image_response":             imageResponse,
		"media":                      []*media.Media{inlineMedia, uriMedia, referenceMedia},
		"metadata":                   protocolMetadata,
		"model_rate_limit":           rateLimit,
		"model_usage":                usage,
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

func representativeCategories() *moderation.Categories {
	return &moderation.Categories{
		Sexual:                      moderation.Verdict{Flagged: true, Score: 0.01},
		Hate:                        moderation.Verdict{Flagged: true, Score: 0.02},
		Harassment:                  moderation.Verdict{Flagged: true, Score: 0.03},
		SelfHarm:                    moderation.Verdict{Flagged: true, Score: 0.04},
		SexualMinors:                moderation.Verdict{Flagged: true, Score: 0.05},
		HateThreatening:             moderation.Verdict{Flagged: true, Score: 0.06},
		ViolenceGraphic:             moderation.Verdict{Flagged: true, Score: 0.07},
		SelfHarmIntent:              moderation.Verdict{Flagged: true, Score: 0.08},
		SelfHarmInstructions:        moderation.Verdict{Flagged: true, Score: 0.09},
		HarassmentThreatening:       moderation.Verdict{Flagged: true, Score: 0.10},
		Violence:                    moderation.Verdict{Flagged: true, Score: 0.11},
		DangerousAndCriminalContent: moderation.Verdict{Flagged: true, Score: 0.12},
		Health:                      moderation.Verdict{Flagged: true, Score: 0.13},
		Financial:                   moderation.Verdict{Flagged: true, Score: 0.14},
		Law:                         moderation.Verdict{Flagged: true, Score: 0.15},
		Pii:                         moderation.Verdict{Flagged: true, Score: 0.16},
		Illicit:                     moderation.Verdict{Flagged: true, Score: 0.17},
		IllicitViolent:              moderation.Verdict{Flagged: true, Score: 0.18},
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

func pointer[T any](value T) *T { return &value }
