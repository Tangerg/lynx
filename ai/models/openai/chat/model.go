package chat

//
//import (
//	"context"
//	"io"
//
//	"github.com/sashabaranov/go-openai"
//
//	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
//	"github.com/Tangerg/lynx/ai/core/chat/model"
//	"github.com/Tangerg/lynx/ai/core/chat/request"
//	"github.com/Tangerg/lynx/ai/core/chat/response"
//	"github.com/Tangerg/lynx/ai/models/openai/api"
//	"github.com/Tangerg/lynx/ai/models/openai/metadata"
//)
//
//type OpenAIChatPrompt = request.ChatRequest[*OpenAIChatOptions]
//
//func newChatPromptBuilder() *request.ChatRequestBuilder[*OpenAIChatOptions] {
//	return request.NewChatRequestBuilder[*OpenAIChatOptions]()
//}
//
//type OpenAIChatCompletion = response.ChatResponse[*metadata.OpenAIChatGenerationMetadata]
//
//func newChatCompletionBuilder() *response.ChatResponseBuilder[*metadata.OpenAIChatGenerationMetadata] {
//	return response.NewChatResponseBuilder[*metadata.OpenAIChatGenerationMetadata]()
//}
//
//var _ model.ChatModel[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata] = (*OpenAIChatModel)(nil)
//
//type OpenAIChatModel struct {
//	helper    *helper
//	config    *OpenAIChatModelConfig
//	openAIApi *api.OpenAIApi
//}
//
//func NewOpenAIChatModel(conf OpenAIChatModelConfig) model.ChatModel[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata] {
//	return &OpenAIChatModel{
//		helper:    &helper{},
//		config:    &conf,
//		openAIApi: api.NewOpenAIApi(conf.Token),
//	}
//}
//
//func (o *OpenAIChatModel) Stream(ctx context.Context, req *OpenAIChatPrompt) (*OpenAIChatCompletion, error) {
//	creq := o.helper.createApiRequest(req, true)
//
//	stream, err := o.openAIApi.CreateChatCompletionStream(ctx, creq)
//	if err != nil {
//		return nil, err
//	}
//	defer func() {
//		_ = stream.Close()
//	}()
//
//	var (
//		openAIChatCompletion *OpenAIChatCompletion
//		recv                 openai.ChatCompletionStreamResponse
//		recvs                = make([]openai.ChatCompletionStreamResponse, 0, 64)
//		streamFunc           = req.Options().StreamChunkFunc()
//		streamCompletionFunc = req.Options().StreamCompletionFunc()
//	)
//
//	for {
//		recv, err = stream.Recv()
//
//		if err != nil {
//			if err == io.EOF {
//				return o.helper.merageStreamResponse(recvs)
//			}
//			return openAIChatCompletion, err
//		}
//
//		if len(recv.Choices) == 0 {
//			continue
//		}
//		recvs = append(recvs, recv)
//		openAIChatCompletion = o.helper.createStreamResponse(&recv)
//
//		if streamFunc != nil {
//			err = streamFunc(ctx, recv.Choices[0].Delta.Content)
//			if err != nil {
//				return openAIChatCompletion, err
//			}
//		}
//
//		if streamCompletionFunc != nil {
//			err = streamCompletionFunc(ctx, openAIChatCompletion)
//			if err != nil {
//				return openAIChatCompletion, err
//			}
//		}
//	}
//}
//
//func (o *OpenAIChatModel) Call(ctx context.Context, req *OpenAIChatPrompt) (*OpenAIChatCompletion, error) {
//	creq := o.helper.createApiRequest(req, false)
//
//	cres, err := o.openAIApi.CreateChatCompletion(ctx, creq)
//	if err != nil {
//		return nil, err
//	}
//
//	resp := o.helper.createCallResponse(&cres)
//
//	if o.helper.IsProxyToolCalls(req.Options(), o.helper.defaultOptions) {
//		return resp, nil
//	}
//
//	if !o.helper.IsToolCallChatCompletion(
//		resp,
//		[]chatMetadata.FinishReason{chatMetadata.ToolCalls, chatMetadata.Stop},
//	) {
//		return resp, nil
//	}
//
//	msgs, err := o.helper.HandleToolCalls(ctx, req, resp)
//	if err != nil {
//		return resp, err
//	}
//	newReq, _ := newChatPromptBuilder().
//		WithOptions(req.Options()).
//		WithMessages(msgs...).
//		Build()
//
//	return o.Call(ctx, newReq)
//}
