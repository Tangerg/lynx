package chat

//
//import (
//	"context"
//	"os"
//	"testing"
//
//	"github.com/sashabaranov/go-openai"
//)
//
//func TestNewOpenAIChatModel(t *testing.T) {
//	token := os.Getenv("OPENAI_TOKEN")
//	t.Log(token)
//	openAIChatModel := NewOpenAIChatModel(OpenAIChatModelConfig{
//		Token: token,
//	})
//	t.Log(openAIChatModel)
//}
//
//func TestOpenAIChatModel_Call(t *testing.T) {
//	token := os.Getenv("OPENAI_TOKEN")
//	t.Log(token)
//	openAIChatModel := NewOpenAIChatModel(OpenAIChatModelConfig{
//		Token: token,
//	})
//
//	opts, _ := NewOpenAIChatOptionsBuilder().
//		WithModel(openai.GPT4oMini).
//		Build()
//
//	prompt, err := newChatPromptBuilder().
//		WithContent("hello! who are you?").
//		WithOptions(opts).
//		Build()
//	if err != nil {
//		t.Log(err)
//		return
//	}
//
//	resp, err := openAIChatModel.Call(context.Background(), prompt)
//	if err != nil {
//		t.Log(err)
//		return
//	}
//	for _, v := range resp.Results() {
//		t.Log(v.Output().Type())
//		t.Log(v.Output().Content())
//		t.Log(v.Metadata().FinishReason())
//	}
//}
//
//func TestOpenAIChatModel_Stream(t *testing.T) {
//	token := os.Getenv("OPENAI_TOKEN")
//	t.Log(token)
//	openAIChatModel := NewOpenAIChatModel(OpenAIChatModelConfig{
//		Token: token,
//	})
//
//	opts, _ := NewOpenAIChatOptionsBuilder().
//		WithModel(openai.GPT4oMini).
//		WithStreamChunkFunc(func(ctx context.Context, chunk string) error {
//			t.Log(chunk)
//			return nil
//		}).
//		WithStreamCompletionFunc(func(ctx context.Context, completion *OpenAIChatCompletion) error {
//			for _, v := range completion.Results() {
//				t.Log(v.Output().Type())
//				t.Log(v.Output().Content())
//				t.Log(v.Metadata().FinishReason())
//			}
//			return nil
//		}).
//		Build()
//
//	prompt, err := newChatPromptBuilder().
//		WithContent("hello! who are you?").
//		WithOptions(opts).
//		Build()
//	if err != nil {
//		t.Log(err)
//		return
//	}
//
//	resp, err := openAIChatModel.Stream(context.Background(), prompt)
//	if err != nil {
//		t.Log(err)
//		return
//	}
//	for _, v := range resp.Results() {
//		t.Log(v.Output().Type())
//		t.Log(v.Output().Content())
//		t.Log(v.Metadata().FinishReason())
//	}
//	t.Log(resp.Result().Metadata().FinishReason())
//	t.Log(resp.Result().Output().Type())
//	t.Log(resp.Result().Output().Content())
//}
