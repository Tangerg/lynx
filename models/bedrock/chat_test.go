package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/pkg/mime"
)

func TestBuildMessages_ImageMedia(t *testing.T) {
	pngMime, err := mime.New("image", "png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := media.NewMedia(pngMime, []byte{0x89, 0x50, 0x4e, 0x47})
	if err != nil {
		t.Fatal(err)
	}
	msg := chat.NewUserMessage(chat.MessageParams{
		Text:  "look at this",
		Media: []*media.Media{img},
	})

	_, msgs := buildMessages([]chat.Message{msg})
	if len(msgs) != 1 || len(msgs[0].Content) != 2 {
		t.Fatalf("want 1 message with 2 blocks (text + image), got %#v", msgs)
	}
	if _, ok := msgs[0].Content[0].(*types.ContentBlockMemberText); !ok {
		t.Errorf("block[0] is %T, want text", msgs[0].Content[0])
	}
	ib, ok := msgs[0].Content[1].(*types.ContentBlockMemberImage)
	if !ok {
		t.Fatalf("block[1] is %T, want image", msgs[0].Content[1])
	}
	if ib.Value.Format != types.ImageFormatPng {
		t.Errorf("format: want png, got %v", ib.Value.Format)
	}
	src, ok := ib.Value.Source.(*types.ImageSourceMemberBytes)
	if !ok {
		t.Fatalf("source type %T, want bytes", ib.Value.Source)
	}
	if len(src.Value) != 4 {
		t.Errorf("payload bytes len=%d, want 4", len(src.Value))
	}
}

func TestBuildMessages_ReasoningRoundTrip(t *testing.T) {
	rp := &chat.ReasoningPart{
		Text:      "thinking out loud",
		Signature: []byte("sig-abc"),
	}
	tp := &chat.TextPart{Text: "and the answer is 42"}
	msg := chat.NewAssistantMessage(chat.MessageParams{
		Parts: []chat.OutputPart{rp, tp},
	})

	_, msgs := buildMessages([]chat.Message{msg})
	if len(msgs) != 1 || len(msgs[0].Content) != 2 {
		t.Fatalf("want 1 message with 2 blocks, got %#v", msgs)
	}
	rc, ok := msgs[0].Content[0].(*types.ContentBlockMemberReasoningContent)
	if !ok {
		t.Fatalf("block[0] is %T, want reasoning content", msgs[0].Content[0])
	}
	rt, ok := rc.Value.(*types.ReasoningContentBlockMemberReasoningText)
	if !ok {
		t.Fatalf("reasoning block %T, want text variant", rc.Value)
	}
	if aws.ToString(rt.Value.Text) != "thinking out loud" {
		t.Errorf("text=%q", aws.ToString(rt.Value.Text))
	}
	if aws.ToString(rt.Value.Signature) != "sig-abc" {
		t.Errorf("signature=%q", aws.ToString(rt.Value.Signature))
	}
}

func TestBuildResponse_ReasoningAndCacheTokens(t *testing.T) {
	cm := &ChatModel{}
	reasoningText := "let me think"
	signature := "sig-xyz"
	input := int32(100)
	output := int32(40)
	cacheRead := int32(80)
	cacheWrite := int32(20)

	out := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberReasoningContent{
						Value: &types.ReasoningContentBlockMemberReasoningText{
							Value: types.ReasoningTextBlock{
								Text:      &reasoningText,
								Signature: &signature,
							},
						},
					},
					&types.ContentBlockMemberText{Value: "final answer"},
				},
			},
		},
		StopReason: types.StopReasonEndTurn,
		Usage: &types.TokenUsage{
			InputTokens:           &input,
			OutputTokens:          &output,
			CacheReadInputTokens:  &cacheRead,
			CacheWriteInputTokens: &cacheWrite,
		},
	}

	resp, err := cm.buildResponse(out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result == nil || resp.Result.AssistantMessage == nil {
		t.Fatal("nil assistant message")
	}
	parts := resp.Result.AssistantMessage.Parts
	if len(parts) != 2 {
		t.Fatalf("want 2 parts, got %d", len(parts))
	}
	rp, ok := parts[0].(*chat.ReasoningPart)
	if !ok {
		t.Fatalf("parts[0] is %T, want reasoning", parts[0])
	}
	if rp.Text != reasoningText || string(rp.Signature) != signature {
		t.Errorf("reasoning round-trip mismatch: %#v", rp)
	}

	u := resp.Metadata.Usage
	if u == nil || u.PromptTokens != 100 || u.CompletionTokens != 40 {
		t.Errorf("usage tokens: %#v", u)
	}
	if u.CacheReadInputTokens == nil || *u.CacheReadInputTokens != 80 {
		t.Errorf("cache read: %v", u.CacheReadInputTokens)
	}
	if u.CacheWriteInputTokens == nil || *u.CacheWriteInputTokens != 20 {
		t.Errorf("cache write: %v", u.CacheWriteInputTokens)
	}
}

func TestToolUseRoundTrip(t *testing.T) {
	input := map[string]any{"city": "Paris", "units": "celsius"}
	args, _ := json.Marshal(input)

	tc := &chat.ToolCallPart{
		ID:        "tc-1",
		Name:      "weather",
		Arguments: string(args),
	}
	msg := chat.NewAssistantMessage(chat.MessageParams{Parts: []chat.OutputPart{tc}})

	_, msgs := buildMessages([]chat.Message{msg})
	if len(msgs) != 1 || len(msgs[0].Content) != 1 {
		t.Fatalf("want 1 message with 1 block, got %#v", msgs)
	}
	tu, ok := msgs[0].Content[0].(*types.ContentBlockMemberToolUse)
	if !ok {
		t.Fatalf("block[0] is %T, want tool use", msgs[0].Content[0])
	}
	if aws.ToString(tu.Value.ToolUseId) != "tc-1" || aws.ToString(tu.Value.Name) != "weather" {
		t.Errorf("tool id/name: %#v", tu.Value)
	}
	if tu.Value.Input == nil {
		t.Fatal("Input is nil — expected populated document")
	}
}

func TestStreamReasoningDelta(t *testing.T) {
	acc := newChunkAccumulator()
	evt := &types.ConverseStreamOutputMemberContentBlockDelta{
		Value: types.ContentBlockDeltaEvent{
			Delta: &types.ContentBlockDeltaMemberReasoningContent{
				Value: &types.ReasoningContentBlockDeltaMemberText{Value: "thinking..."},
			},
		},
	}
	resp, ok := acc.AddChunk(evt)
	if !ok {
		t.Fatal("expected reasoning delta to produce a response")
	}
	parts := resp.Result.AssistantMessage.Parts
	if len(parts) != 1 {
		t.Fatalf("want 1 part, got %d", len(parts))
	}
	rp, ok := parts[0].(*chat.ReasoningPart)
	if !ok {
		t.Fatalf("parts[0] is %T, want reasoning", parts[0])
	}
	if rp.Text != "thinking..." {
		t.Errorf("reasoning text=%q", rp.Text)
	}
}
