package chatconformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

type mappingCase struct {
	provider string
	assert   func(*testing.T, *chat.Request, *chat.Response)
}

func TestReferenceProviderMappings(t *testing.T) {
	cases := []mappingCase{
		{provider: "openai", assert: assertOpenAI},
		{provider: "anthropic", assert: assertAnthropic},
		{provider: "google", assert: assertGoogle},
		{provider: "ollama", assert: assertOllama},
	}

	for _, test := range cases {
		t.Run(test.provider, func(t *testing.T) {
			request := loadFixture[chat.Request](t, test.provider+".request.golden.json")
			response := loadFixture[chat.Response](t, test.provider+".response.golden.json")
			assertFixedPoint(t, request)
			assertFixedPoint(t, response)
			assertConversationShape(t, request)
			test.assert(t, request, response)
		})
	}
}

func loadFixture[T any](t *testing.T, name string) *T {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	return &value
}

type validatable interface {
	Validate() error
}

func assertFixedPoint[T any](t *testing.T, value *T) {
	t.Helper()
	valid, ok := any(value).(validatable)
	if !ok {
		t.Fatalf("%T does not implement Validate", value)
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate %T: %v", value, err)
	}
	first, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal %T: %v", value, err)
	}
	var decoded T
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("Unmarshal %T: %v", value, err)
	}
	second, err := json.Marshal(&decoded)
	if err != nil {
		t.Fatalf("second Marshal %T: %v", value, err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("%T wire is not a fixed point\nfirst:  %s\nsecond: %s", value, first, second)
	}
}

func assertConversationShape(t *testing.T, request *chat.Request) {
	t.Helper()
	roles := make([]chat.Role, 0, len(request.Messages))
	for i := range request.Messages {
		roles = append(roles, request.Messages[i].Role)
	}
	for _, role := range []chat.Role{chat.RoleSystem, chat.RoleUser, chat.RoleAssistant, chat.RoleTool} {
		if !slices.Contains(roles, role) {
			t.Fatalf("request roles %v do not contain %q", roles, role)
		}
	}
	if len(request.Tools) == 0 {
		t.Fatal("request does not advertise a tool definition")
	}
}

func assertOpenAI(t *testing.T, request *chat.Request, response *chat.Response) {
	t.Helper()
	if request.Options.TopK != nil {
		t.Fatal("OpenAI fixture unexpectedly maps unsupported top_k")
	}
	native := requireExtension[map[string]json.RawMessage](t, request.Extensions, "openai/request")
	for _, key := range []string{"response_format", "modalities", "audio"} {
		if _, ok := native[key]; !ok {
			t.Fatalf("openai/request does not preserve %q", key)
		}
	}
	if len(response.Choices) != 2 {
		t.Fatalf("OpenAI choices = %d, want 2", len(response.Choices))
	}
	wantKinds := []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall, chat.PartMedia}
	if got := partKinds(response.Choices[0].Message); !slices.Equal(got, wantKinds) {
		t.Fatalf("OpenAI choice parts = %v, want %v", got, wantKinds)
	}
	if response.Choices[0].FinishReason != chat.FinishReasonToolCalls || response.Choices[1].FinishReason != chat.FinishReasonContentFilter {
		t.Fatalf("OpenAI finish reasons = %q, %q", response.Choices[0].FinishReason, response.Choices[1].FinishReason)
	}
	if response.Usage.ReasoningTokens == nil || *response.Usage.ReasoningTokens != 3 {
		t.Fatalf("OpenAI reasoning usage = %v", response.Usage.ReasoningTokens)
	}
	if response.Usage.CacheReadInputTokens == nil || *response.Usage.CacheReadInputTokens != 5 {
		t.Fatalf("OpenAI cache usage = %v", response.Usage.CacheReadInputTokens)
	}
	mediaPart := response.Choices[0].Message.Parts[3]
	if mediaPart.Media == nil || mediaPart.Media.Source.Kind != media.SourceReference {
		t.Fatalf("OpenAI audio mapping = %#v", mediaPart.Media)
	}
	requireExtension[string](t, response.Extensions, "openai/service_tier")
	requireExtension[[]map[string]any](t, response.Choices[0].Extensions, "openai/logprobs")
}

func assertAnthropic(t *testing.T, request *chat.Request, response *chat.Response) {
	t.Helper()
	native := requireExtension[map[string]json.RawMessage](t, request.Extensions, "anthropic/request")
	for _, key := range []string{"thinking", "cache_control"} {
		if _, ok := native[key]; !ok {
			t.Fatalf("anthropic/request does not preserve %q", key)
		}
	}
	assistant := response.First().Message
	wantKinds := []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall}
	if got := partKinds(assistant); !slices.Equal(got, wantKinds) {
		t.Fatalf("Anthropic parts = %v, want %v", got, wantKinds)
	}
	if got := string(assistant.Parts[0].Signature); got != "sig-anthropic" {
		t.Fatalf("Anthropic thinking signature = %q", got)
	}
	if got := requireExtension[string](t, assistant.Metadata, "anthropic/redacted_reasoning"); got != "opaque-redacted-block" {
		t.Fatalf("Anthropic redacted reasoning = %q", got)
	}
	if response.Usage.CacheReadInputTokens == nil || *response.Usage.CacheReadInputTokens != 40 {
		t.Fatalf("Anthropic cache-read usage = %v", response.Usage.CacheReadInputTokens)
	}
	if response.Usage.CacheWriteInputTokens == nil || *response.Usage.CacheWriteInputTokens != 20 {
		t.Fatalf("Anthropic cache-write usage = %v", response.Usage.CacheWriteInputTokens)
	}
	toolResult := request.Messages[len(request.Messages)-1].Parts[0].ToolResult
	if toolResult == nil || !toolResult.IsError {
		t.Fatalf("Anthropic tool error mapping = %#v", toolResult)
	}
	requireExtension[string](t, response.Extensions, "anthropic/stop_sequence")
}

func assertGoogle(t *testing.T, request *chat.Request, response *chat.Response) {
	t.Helper()
	native := requireExtension[map[string]json.RawMessage](t, request.Extensions, "google/request")
	for _, key := range []string{"safety_settings", "response_modalities"} {
		if _, ok := native[key]; !ok {
			t.Fatalf("google/request does not preserve %q", key)
		}
	}
	if len(response.Choices) != 2 {
		t.Fatalf("Google choices = %d, want 2", len(response.Choices))
	}
	parts := response.Choices[0].Message.Parts
	if got := partKinds(response.Choices[0].Message); !slices.Equal(got, []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall}) {
		t.Fatalf("Google parts = %v", got)
	}
	if string(parts[0].Signature) != "sig-google" {
		t.Fatalf("Google thought signature = %q", parts[0].Signature)
	}
	if parts[2].ToolCall == nil || !strings.HasPrefix(parts[2].ToolCall.ID, "google/") {
		t.Fatalf("Google synthesized tool-call ID = %#v", parts[2].ToolCall)
	}
	if response.Usage.ReasoningTokens == nil || *response.Usage.ReasoningTokens != 4 {
		t.Fatalf("Google thoughts usage = %v", response.Usage.ReasoningTokens)
	}
	if response.Usage.CacheReadInputTokens == nil || *response.Usage.CacheReadInputTokens != 6 {
		t.Fatalf("Google cached usage = %v", response.Usage.CacheReadInputTokens)
	}
	requireExtension[int64](t, response.Extensions, "google/tool_use_prompt_tokens")
	requireExtension[[]map[string]any](t, response.Choices[1].Extensions, "google/safety_ratings")
}

func assertOllama(t *testing.T, request *chat.Request, response *chat.Response) {
	t.Helper()
	native := requireExtension[map[string]json.RawMessage](t, request.Extensions, "ollama/request")
	for _, key := range []string{"keep_alive", "format", "think", "options"} {
		if _, ok := native[key]; !ok {
			t.Fatalf("ollama/request does not preserve %q", key)
		}
	}
	if len(response.Choices) != 1 {
		t.Fatalf("Ollama choices = %d, want 1", len(response.Choices))
	}
	wantKinds := []chat.PartKind{chat.PartReasoning, chat.PartText, chat.PartToolCall}
	if got := partKinds(response.First().Message); !slices.Equal(got, wantKinds) {
		t.Fatalf("Ollama parts = %v, want %v", got, wantKinds)
	}
	if response.Usage.InputTokens != 11 || response.Usage.OutputTokens != 5 {
		t.Fatalf("Ollama usage = %#v", response.Usage)
	}
	durations := requireExtension[map[string]int64](t, response.Extensions, "ollama/durations_ms")
	if durations["total"] != 1250 || durations["eval"] != 700 {
		t.Fatalf("Ollama durations = %#v", durations)
	}
	if got := requireExtension[string](t, response.First().Extensions, "ollama/native_done_reason"); got != "stop" {
		t.Fatalf("Ollama done reason = %q", got)
	}
}

func partKinds(message *chat.Message) []chat.PartKind {
	if message == nil {
		return nil
	}
	kinds := make([]chat.PartKind, len(message.Parts))
	for i := range message.Parts {
		kinds[i] = message.Parts[i].Kind
	}
	return kinds
}

func requireExtension[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, ok, err := metadata.Decode[T](values, key)
	if err != nil {
		t.Fatalf("decode extension %q: %v", key, err)
	}
	if !ok {
		t.Fatalf("missing extension %q", key)
	}
	return value
}
