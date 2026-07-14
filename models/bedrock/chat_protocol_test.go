package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func TestChatBuildConverseInput(t *testing.T) {
	temperature := 0.4
	image, err := media.NewBytes("image/png", []byte("png"))
	if err != nil {
		t.Fatal(err)
	}
	request := &corechat.Request{
		Messages: []corechat.Message{
			corechat.NewSystemMessage("system"),
			corechat.NewUserMessage(corechat.NewTextPart("look"), corechat.NewMediaPart(image)),
			corechat.NewAssistantMessage(
				corechat.NewReasoningPart("thinking", []byte("sig")),
				corechat.NewToolCallPart(corechat.ToolCall{ID: "call-1", Name: "weather", Arguments: `{"city":"Paris"}`}),
			),
			corechat.NewToolMessage(corechat.ToolResult{ID: "call-1", Name: "weather", Result: "rain", IsError: true}),
		},
		Tools: []corechat.ToolDefinition{{
			Name: "weather", Description: "Get weather", InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Options: corechat.Options{Temperature: &temperature},
	}
	if err := request.SetExtension(ChatRequestExtensionKey, ChatRequestOptions{
		AdditionalModelRequestFields: map[string]any{"thinking": true},
		RequestMetadata:              map[string]string{"tenant": "test"},
	}); err != nil {
		t.Fatal(err)
	}

	model := &Chat{api: &API{}, defaults: corechat.Options{Model: "anthropic.claude-test"}}
	input, modelName, err := model.buildConverseInput(request)
	if err != nil {
		t.Fatal(err)
	}
	if modelName != "anthropic.claude-test" || aws.ToString(input.ModelId) != modelName {
		t.Fatalf("model = %q / %q", modelName, aws.ToString(input.ModelId))
	}
	if len(input.System) != 1 || len(input.Messages) != 3 {
		t.Fatalf("system/messages = %d/%d", len(input.System), len(input.Messages))
	}
	if input.InferenceConfig == nil || input.InferenceConfig.Temperature == nil || *input.InferenceConfig.Temperature != 0.4 {
		t.Fatalf("inference = %#v", input.InferenceConfig)
	}
	if input.ToolConfig == nil || len(input.ToolConfig.Tools) != 1 {
		t.Fatalf("tools = %#v", input.ToolConfig)
	}
	if input.RequestMetadata["tenant"] != "test" || input.AdditionalModelRequestFields == nil {
		t.Fatalf("native options = %#v", input)
	}
	toolResult, ok := input.Messages[2].Content[0].(*types.ContentBlockMemberToolResult)
	if !ok || toolResult.Value.Status != types.ToolResultStatusError {
		t.Fatalf("tool result = %#v", input.Messages[2].Content[0])
	}
}

func TestMapProtocolConverseResponse(t *testing.T) {
	output := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{Value: types.Message{
			Role: types.ConversationRoleAssistant,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberReasoningContent{Value: &types.ReasoningContentBlockMemberReasoningText{Value: types.ReasoningTextBlock{Text: aws.String("think"), Signature: aws.String("sig")}}},
				&types.ContentBlockMemberText{Value: "answer"},
				&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{ToolUseId: aws.String("call-1"), Name: aws.String("weather"), Input: toDocument(map[string]any{"city": "Paris"})}},
			},
		}},
		StopReason: types.StopReasonToolUse,
		Usage: &types.TokenUsage{
			InputTokens: aws.Int32(11), OutputTokens: aws.Int32(7), CacheReadInputTokens: aws.Int32(3),
		},
	}

	response, err := mapProtocolConverseResponse("model", output)
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "model" || response.First().FinishReason != corechat.FinishReasonToolCalls {
		t.Fatalf("response = %#v", response)
	}
	wantKinds := []corechat.PartKind{corechat.PartReasoning, corechat.PartText, corechat.PartToolCall}
	parts := response.First().Message.Parts
	for index, want := range wantKinds {
		if parts[index].Kind != want {
			t.Fatalf("part[%d] = %q, want %q", index, parts[index].Kind, want)
		}
	}
	if response.Usage.InputTokens != 11 || response.Usage.OutputTokens != 7 || response.Usage.CacheReadInputTokens == nil || *response.Usage.CacheReadInputTokens != 3 {
		t.Fatalf("usage = %#v", response.Usage)
	}
}

func TestProtocolChunkAccumulatorRetainsToolIdentity(t *testing.T) {
	accumulator := newProtocolChunkAccumulator("model")
	index := int32(2)
	start := &types.ConverseStreamOutputMemberContentBlockStart{Value: types.ContentBlockStartEvent{
		ContentBlockIndex: &index,
		Start: &types.ContentBlockStartMemberToolUse{Value: types.ToolUseBlockStart{
			ToolUseId: aws.String("call-1"), Name: aws.String("weather"),
		}},
	}}
	response, include, err := accumulator.add(start)
	if err != nil || !include || response.First().Message.Parts[0].ToolCall.Name != "weather" {
		t.Fatalf("start = %#v, %v, %v", response, include, err)
	}

	arguments := `{"city":"Paris"}`
	delta := &types.ConverseStreamOutputMemberContentBlockDelta{Value: types.ContentBlockDeltaEvent{
		ContentBlockIndex: &index,
		Delta:             &types.ContentBlockDeltaMemberToolUse{Value: types.ToolUseBlockDelta{Input: &arguments}},
	}}
	response, include, err = accumulator.add(delta)
	if err != nil || !include {
		t.Fatalf("delta = %#v, %v, %v", response, include, err)
	}
	call := response.First().Message.Parts[0].ToolCall
	if call.ID != "call-1" || call.Name != "weather" || call.Arguments != arguments {
		t.Fatalf("tool call = %#v", call)
	}
}
