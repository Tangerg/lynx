package openai

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/extensions/tools/fakeweatherquery"
	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/mime"
)

const (
	testImageBase64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGAAAABhCAYAAAApKSdAAAACXBIWXMAACE4AAAhOAFFljFgAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAUUSURBVHgB7Z29bhtHFIWPHQN2J7lKqnhYpYvpIukCbJEAKQJEegLReYFIT0DrCSI9QEDqCSIDaQIEIOukiJwyza5SJWlId3FFz+HuGmuSSw6p+dlZ3g84luhdUeI9M3fmziyXgBCUe/DHYY0Wj/tgWmjV42zFcWe4MIBBPNJ6qqW0uvAbXFvQgKzQK62bQhkaCIPc10q1Zi3XH1o/IG9cwUm0RogrgDY1KmLgHYX9DvyiBvDYI77XmiD+oLlQHw7hIDoCMBOt1U9w0BsU9mOAtaUUFk3oQoIfzAQFCf5dNMEdTFCQ4NtQih1NSIGgf3ibxOJt5UrAB1gNK72vIdjiI61HWr+YnNxDXK0rJiULsV65GJeiIuscLSTTeobKSutiCuojX8kU3MBx4I3WeNVBBRl4fWiCyoB8v2JAAkk9PmDwT8sH1TEghRjgC27scCx41wO43KAg+ILxTvhNaUACwTc04Z0B30LwzTzm5Rjw3sgseIG1wGMawMBPIOQcqvzrNIMHOg9Q5KK953O90/rFC+BhJRH8PQZ+fu7SjC7HAIV95yu99vjlxfvBJx8nwHd6IfNJAkccOjHg6OgIs9lsra6vr2GTNE03/k7q8HAhyJ/2gM9O65/4kT7/mwEcoZwYsPQiV3BwcABb9Ho9KKU2njccDjGdLlxx+InBBPBAAR86ydRPaIC9SASi3+8bnXd+fr78nw8NJ39uDJjXAVFPP7dp/VmWLR9g6w6Huo/IOTk5MTpvZesn/93AiP/dXCwd9SyILT9Jko3n1bZ+8s8rGPGvoVHbEXcPMM39V1dX9Qd/19PPNxta959D4HUGF0RrAFs/8/8mxuPxXLUwtfx2WX+cxdivZ3DFA0SKldZPuPTAKrikbOlMOX+9zFu/Q2iAQoSY5H7mfeb/tXCT8MdneU9wNNCuQUXZA0ynnrUznyqOcrspUY4BJunHqPU3gOgMsNr6G0B0BpgUXrG0fhKVAaaF1/HxMWIhKgNMcj9Tz82Nk6rVGdav/tJ5eraJ0Wi01XPq1r/xOS8uLkJc6XYnRTMNXdf62eIvLy+jyftVghnQ7Xahe8FW59fBTRYOzosDNI1hJdz0lBQkBflkMBjMU5iL13pXRb8fYAJrB/a2db0oFHthAOEUliaYFHE+aaUBdZsvvFhApyM0idYZwOCvW4JmIWdSzPmidQaYrAGZ7iX4oFUGnJ2dGdUCTRqMozeANQCLsE6nA10JG/0Mx4KmDMbBCjEWR2yxu8LAM98vXulmCA2ovVLCI8EMYODWbpbvCXtTBzQVMSAwYkBgxIDAtNKAXWdGIRADAiMpKDA0IIMQikx6QGDEgMCIAYGRMSAsMgaEhgbcQgjFa+kBYZnIGBCWWzEgLPNBOJ6Fk/aR8Y5ZCvktKwX/PJZ7xoVjfs+4chYU11tK2sE85qUBLyH4Zh5z6QHhGPOf6r2j+TEbcgdFP2RaHX5TrYQlDflj5RXE5Q1cG/lWnhYpReUGKdUewGnRmhvnCJbgmxey8sHiZ8iwF3AsUBBckKHI/SWLq6HsBc8huML4DiK80D6WnBqLzN68UFCmopheYJOVYgcU5FOVbAVfYUcUZGoaLPglCtITdg2+tZUFBTFh2+ArWEYh/7z0WIIQSiM43lt5AWAmWhLHylN4QmkNEXfAbGqEQKsHSfHLYwiSq8AnaAAKeaW3D8VbijwNW5nh3IN9FPI/jnpaPKZi2/SfFuJu4W3x9RqWL+N5C+7ruKpBAgLkAAAAAElFTkSuQmCC"
	testImageURL    = "https://upload.wikimedia.org/wikipedia/commons/thumb/d/dd/Gfp-wisconsin-madison-the-nature-boardwalk.jpg/2560px-Gfp-wisconsin-madison-the-nature-boardwalk.jpg"
	weatherQuery    = "I want to inquire about the weather conditions in Beijing on May 1st, 2023"
	visionQuery     = "Please describe this picture"
)

func newTestChatModel(t *testing.T) *ChatModel {
	t.Helper()

	defaultOptions := assert.Must(chat.NewOptions(currentConfig.chatModel))

	model, err := NewChatModel(
		getAPIKey(t),
		defaultOptions,
		option.WithBaseURL(currentConfig.baseURL),
	)
	if err != nil {
		t.Fatalf("failed to create chat model: %v", err)
	}

	return model
}

func TestNewChatModel(t *testing.T) {
	tests := []struct {
		name           string
		apiKey         model.ApiKey
		defaultOptions *chat.Options
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "valid configuration",
			apiKey:         getAPIKey(t),
			defaultOptions: assert.Must(chat.NewOptions(currentConfig.chatModel)),
			wantErr:        false,
		},
		{
			name:           "nil api key",
			apiKey:         nil,
			defaultOptions: assert.Must(chat.NewOptions(currentConfig.chatModel)),
			wantErr:        true,
			errMsg:         "apiKey is required",
		},
		{
			name:    "nil default options",
			apiKey:  getAPIKey(t),
			wantErr: true,
			errMsg:  "default options cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := NewChatModel(
				tt.apiKey,
				tt.defaultOptions,
				option.WithBaseURL(currentConfig.baseURL),
			)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error message %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if model == nil {
				t.Error("expected non-nil model")
			}

			if model.defaultOptions == nil {
				t.Error("expected non-nil default options")
			}
		})
	}
}

func TestChatModel_Call(t *testing.T) {
	model := newTestChatModel(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		messages []chat.Message
		options  *chat.Options
		wantErr  bool
	}{
		{
			name: "simple user message",
			messages: []chat.Message{
				chat.NewUserMessage("Hello! Please respond with 'Hi there!'"),
			},
			wantErr: false,
		},
		{
			name: "multi-turn conversation",
			messages: []chat.Message{
				chat.NewSystemMessage("You are a helpful assistant."),
				chat.NewUserMessage("What is 2+2?"),
				chat.NewAssistantMessage("2+2 equals 4."),
				chat.NewUserMessage("What about 3+3?"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := chat.NewRequest(tt.messages)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			if tt.options != nil {
				req.Options = tt.options
			}

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if result.AssistantMessage == nil {
				t.Fatal("expected non-nil assistant message")
			}

			responseText := result.AssistantMessage.Text
			t.Logf("Response: %s", responseText)

			if resp.Metadata == nil {
				t.Error("expected non-nil metadata")
			}
		})
	}
}

func TestChatModel_Stream(t *testing.T) {
	model := newTestChatModel(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		messages []chat.Message
		options  *chat.Options
		wantErr  bool
	}{
		{
			name: "simple streaming",
			messages: []chat.Message{
				chat.NewUserMessage("Count from 1 to 5 slowly."),
			},
			wantErr: false,
		},
		{
			name: "streaming with system message",
			messages: []chat.Message{
				chat.NewSystemMessage("You are a helpful assistant."),
				chat.NewUserMessage("Say hello and introduce yourself."),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := chat.NewRequest(tt.messages)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			if tt.options != nil {
				req.Options = tt.options
			}

			stream := model.Stream(ctx, req)
			chunkCount := 0
			accumulator := chat.NewResponseAccumulator()

			for resp, err := range stream {
				if err != nil && !tt.wantErr {
					t.Fatalf("unexpected stream error: %v", err)
				}

				if tt.wantErr {
					return
				}

				if resp == nil {
					t.Error("expected non-nil response")
					continue
				}

				result := resp.Result()
				if result == nil || result.AssistantMessage == nil {
					continue
				}

				chunkCount++
				accumulator.AddChunk(resp)

				responseText := result.AssistantMessage.Text
				t.Logf("Chunk %d: %s", chunkCount, responseText)
			}

			if !tt.wantErr && chunkCount == 0 {
				t.Error("expected at least one chunk")
			}

			finalResponse := &accumulator.Response
			if finalResponse.Metadata == nil {
				t.Error("expected non-nil metadata in accumulated response")
			}

			finalResult := finalResponse.Result()
			if finalResult != nil && finalResult.AssistantMessage != nil {
				fullText := finalResult.AssistantMessage.Text
				t.Logf("Accumulated full text: %s", fullText)

				if fullText == "" {
					t.Error("expected non-empty accumulated text")
				}
			}

			t.Logf("Total chunks received: %d", chunkCount)
		})
	}
}

func TestChatModel_Call_Tool(t *testing.T) {
	model := newTestChatModel(t)
	ctx := context.Background()

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	toolOptions := assert.Must(chat.NewOptions(currentConfig.chatModel))
	toolOptions.Tools = []chat.Tool{weatherTool}

	tests := []struct {
		name          string
		messages      []chat.Message
		options       *chat.Options
		wantErr       bool
		expectToolUse bool
	}{
		{
			name: "weather query with tool - should return tool call",
			messages: []chat.Message{
				chat.NewUserMessage(weatherQuery),
			},
			options:       toolOptions,
			wantErr:       false,
			expectToolUse: true,
		},
		{
			name: "non-tool query with tool available - should not use tool",
			messages: []chat.Message{
				chat.NewUserMessage("What is the capital of France?"),
			},
			options:       toolOptions,
			wantErr:       false,
			expectToolUse: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := chat.NewRequest(tt.messages)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.Options = tt.options

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			toolCalls := result.AssistantMessage.ToolCalls

			if tt.expectToolUse {
				if len(toolCalls) == 0 {
					t.Error("expected tool calls but got none")
				} else {
					t.Logf("✅ Model returned %d tool call(s)", len(toolCalls))
					for i, tc := range toolCalls {
						t.Logf("  Tool call %d:", i+1)
						t.Logf("    ID: %s", tc.ID)
						t.Logf("    Name: %s", tc.Name)
						t.Logf("    Arguments: %s", tc.Arguments)

						if tc.Name != weatherTool.Definition().Name {
							t.Errorf("expected tool name %q, got %q",
								weatherTool.Definition().Name, tc.Name)
						}

						if tc.ID == "" {
							t.Error("tool call ID should not be empty")
						}
						if tc.Arguments == "" {
							t.Error("tool call arguments should not be empty")
						}
					}
				}

				if result.Metadata.FinishReason != chat.FinishReasonToolCalls {
					t.Errorf("expected finish reason %q, got %q",
						chat.FinishReasonToolCalls, result.Metadata.FinishReason)
				}
			} else {
				if len(toolCalls) > 0 {
					t.Logf("⚠️ Unexpected tool calls (model decided to use tools):")
					for i, tc := range toolCalls {
						t.Logf("  Tool call %d: %s", i+1, tc.Name)
					}
				} else {
					t.Logf("✅ No tool calls as expected")
				}

				responseText := result.AssistantMessage.Text
				if responseText == "" && len(toolCalls) == 0 {
					t.Error("expected either text response or tool calls")
				}
				t.Logf("Response: %s", responseText)
			}
		})
	}
}

func TestChatModel_Stream_Tool(t *testing.T) {
	model := newTestChatModel(t)
	ctx := context.Background()

	weatherTool := fakeweatherquery.NewFakeWeatherQuery(nil)
	toolOptions := assert.Must(chat.NewOptions(currentConfig.chatModel))
	toolOptions.Tools = []chat.Tool{weatherTool}

	tests := []struct {
		name          string
		messages      []chat.Message
		options       *chat.Options
		wantErr       bool
		expectToolUse bool
	}{
		{
			name: "streaming weather query with tool - should return tool call",
			messages: []chat.Message{
				chat.NewUserMessage(weatherQuery),
			},
			options:       toolOptions,
			wantErr:       false,
			expectToolUse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := chat.NewRequest(tt.messages)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.Options = tt.options

			stream := model.Stream(ctx, req)
			chunkCount := 0
			accumulator := chat.NewResponseAccumulator()

			for resp, err := range stream {
				if err != nil && !tt.wantErr {
					t.Fatalf("unexpected stream error: %v", err)
				}

				if tt.wantErr {
					return
				}

				if resp == nil {
					continue
				}

				result := resp.Result()
				if result == nil || result.AssistantMessage == nil {
					continue
				}

				chunkCount++
				accumulator.AddChunk(resp)

				toolCalls := result.AssistantMessage.ToolCalls
				if len(toolCalls) > 0 {
					t.Logf("Chunk %d - Tool calls: %d", chunkCount, len(toolCalls))
					for i, tc := range toolCalls {
						t.Logf("  Tool call %d: ID=%s, Name=%s", i+1, tc.ID, tc.Name)
						if tc.Arguments != "" {
							t.Logf("  Arguments: %s", tc.Arguments)
						}
					}
				}

				responseText := result.AssistantMessage.Text
				if responseText != "" {
					t.Logf("Chunk %d - Text: %s", chunkCount, responseText)
				}
			}

			finalResponse := &accumulator.Response
			finalResult := finalResponse.Result()

			if tt.expectToolUse {
				finalToolCalls := finalResult.AssistantMessage.ToolCalls
				if len(finalToolCalls) == 0 {
					t.Error("expected tool calls in accumulated response but got none")
				} else {
					t.Logf("✅ Accumulated response contains %d tool call(s)", len(finalToolCalls))
					for i, tc := range finalToolCalls {
						t.Logf("  Accumulated tool call %d:", i+1)
						t.Logf("    ID: %s", tc.ID)
						t.Logf("    Name: %s", tc.Name)
						t.Logf("    Arguments: %s", tc.Arguments)

						if tc.Name != weatherTool.Definition().Name {
							t.Errorf("expected tool name %q, got %q",
								weatherTool.Definition().Name, tc.Name)
						}

						if tc.ID == "" {
							t.Error("accumulated tool call ID should not be empty")
						}
						if tc.Arguments == "" {
							t.Error("accumulated tool call arguments should not be empty")
						}
					}

					if finalResult.Metadata.FinishReason != chat.FinishReasonToolCalls {
						t.Errorf("expected finish reason %q, got %q",
							chat.FinishReasonToolCalls, finalResult.Metadata.FinishReason)
					}
				}
			}

			t.Logf("Total chunks: %d", chunkCount)
		})
	}
}

func TestChatModel_Call_Vision_Base64(t *testing.T) {
	model := newTestChatModel(t)
	ctx := context.Background()

	mimeType := mime.NewBuilder().
		WithType("image").
		WithSubType("png").
		MustBuild()

	mediaContent, err := media.NewMedia(mimeType, testImageBase64)
	if err != nil {
		t.Fatalf("failed to create media: %v", err)
	}

	tests := []struct {
		name    string
		message chat.MessageParams
		wantErr bool
	}{
		{
			name: "vision with base64 image",
			message: chat.MessageParams{
				Text:  visionQuery,
				Media: []*media.Media{mediaContent},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []chat.Message{
				chat.NewUserMessage(tt.message),
			}

			req, err := chat.NewRequest(messages)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			responseText := result.AssistantMessage.Text
			if responseText == "" {
				t.Error("expected non-empty response text")
			}

			t.Logf("Vision response: %s", responseText)
		})
	}
}

func TestChatModel_Call_Vision_URL(t *testing.T) {
	model := newTestChatModel(t)
	ctx := context.Background()

	mimeType := mime.NewBuilder().
		WithType("image").
		WithSubType("png").
		MustBuild()

	mediaContent, err := media.NewMedia(mimeType, testImageURL)
	if err != nil {
		t.Fatalf("failed to create media: %v", err)
	}

	tests := []struct {
		name    string
		message chat.MessageParams
		wantErr bool
	}{
		{
			name: "vision with image URL",
			message: chat.MessageParams{
				Text:  visionQuery,
				Media: []*media.Media{mediaContent},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []chat.Message{
				chat.NewUserMessage(tt.message),
			}

			req, err := chat.NewRequest(messages)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := model.Call(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			result := resp.Result()
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			responseText := result.AssistantMessage.Text
			if responseText == "" {
				t.Error("expected non-empty response text")
			}

			t.Logf("Vision response: %s", responseText)
		})
	}
}
