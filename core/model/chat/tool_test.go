package chat

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTool(t *testing.T) {
	t.Run("external tool without exec func", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: `{"type":"object"}`,
		}
		meta := ToolMetadata{ReturnDirect: false}

		tool, err := NewTool(def, meta, nil)
		require.NoError(t, err)
		assert.NotNil(t, tool)

		_, ok := tool.(CallableTool)
		assert.False(t, ok, "should be external tool")

		assert.Equal(t, "test_tool", tool.Definition().Name)
		assert.Equal(t, "A test tool", tool.Definition().Description)
		assert.False(t, tool.Metadata().ReturnDirect)
	})

	t.Run("internal tool with exec func", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "calc",
			Description: "Calculator",
			InputSchema: `{"type":"object"}`,
		}
		meta := ToolMetadata{ReturnDirect: false}
		execFunc := func(ctx context.Context, args string) (string, error) {
			return "42", nil
		}

		tool, err := NewTool(def, meta, execFunc)
		require.NoError(t, err)
		assert.NotNil(t, tool)

		callable, ok := tool.(CallableTool)
		assert.True(t, ok, "should be callable tool")

		result, err := callable.Call(context.Background(), "{}")
		require.NoError(t, err)
		assert.Equal(t, "42", result)
	})

	t.Run("empty tool name", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "",
			InputSchema: `{"type":"object"}`,
		}

		tool, err := NewTool(def, ToolMetadata{}, nil)
		assert.Error(t, err)
		assert.Nil(t, tool)
		assert.Contains(t, err.Error(), "tool name cannot be empty")
	})

	t.Run("empty input schema", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "test",
			InputSchema: "",
		}

		tool, err := NewTool(def, ToolMetadata{}, nil)
		assert.Error(t, err)
		assert.Nil(t, tool)
		assert.Contains(t, err.Error(), "tool input schema cannot be empty")
	})
}

func TestCallableTool_Call(t *testing.T) {
	t.Run("successful call", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "echo",
			Description: "Echo tool",
			InputSchema: `{"type":"object"}`,
		}
		execFunc := func(ctx context.Context, args string) (string, error) {
			return "echoed: " + args, nil
		}

		tool, err := NewTool(def, ToolMetadata{}, execFunc)
		require.NoError(t, err)

		callable := tool.(CallableTool)
		result, err := callable.Call(context.Background(), "test")
		require.NoError(t, err)
		assert.Equal(t, "echoed: test", result)
	})

	t.Run("call with error", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "error_tool",
			InputSchema: `{"type":"object"}`,
		}
		execFunc := func(ctx context.Context, args string) (string, error) {
			return "", errors.New("execution failed")
		}

		tool, err := NewTool(def, ToolMetadata{}, execFunc)
		require.NoError(t, err)

		callable := tool.(CallableTool)
		result, err := callable.Call(context.Background(), "")
		assert.Error(t, err)
		assert.Empty(t, result)
		assert.Contains(t, err.Error(), "execution failed")
	})

	t.Run("call with context", func(t *testing.T) {
		def := ToolDefinition{
			Name:        "context_tool",
			InputSchema: `{"type":"object"}`,
		}
		execFunc := func(ctx context.Context, args string) (string, error) {
			if ctx == nil {
				return "", errors.New("context is nil")
			}
			return "success", nil
		}

		tool, err := NewTool(def, ToolMetadata{}, execFunc)
		require.NoError(t, err)

		callable := tool.(CallableTool)
		result, err := callable.Call(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "success", result)
	})
}

func TestToolRegistry_Register(t *testing.T) {
	t.Run("register single tool", func(t *testing.T) {
		registry := newToolRegistry()
		tool, _ := NewTool(
			ToolDefinition{Name: "tool1", InputSchema: "{}"},
			ToolMetadata{},
			nil,
		)

		registry.Register(tool)

		assert.Equal(t, 1, registry.Size())
		assert.True(t, registry.Exists("tool1"))
	})

	t.Run("register multiple tools", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)

		registry.Register(tool1, tool2)

		assert.Equal(t, 2, registry.Size())
		assert.True(t, registry.Exists("tool1"))
		assert.True(t, registry.Exists("tool2"))
	})

	t.Run("register duplicate tool", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{ReturnDirect: true}, nil)

		registry.Register(tool1)
		registry.Register(tool2)

		assert.Equal(t, 1, registry.Size())
		found, _ := registry.Find("tool1")
		assert.False(t, found.Metadata().ReturnDirect, "should keep first registration")
	})

	t.Run("register nil tool", func(t *testing.T) {
		registry := newToolRegistry()
		registry.Register(nil)

		assert.Equal(t, 0, registry.Size())
	})

	t.Run("register with no tools", func(t *testing.T) {
		registry := newToolRegistry()
		result := registry.Register()

		assert.Equal(t, registry, result)
		assert.Equal(t, 0, registry.Size())
	})

	t.Run("method chaining", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)

		result := registry.Register(tool1).Register(tool2)

		assert.Equal(t, registry, result)
		assert.Equal(t, 2, registry.Size())
	})
}

func TestToolRegistry_Unregister(t *testing.T) {
	t.Run("unregister existing tool", func(t *testing.T) {
		registry := newToolRegistry()
		tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool)

		registry.Unregister("tool1")

		assert.Equal(t, 0, registry.Size())
		assert.False(t, registry.Exists("tool1"))
	})

	t.Run("unregister multiple tools", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool1, tool2)

		registry.Unregister("tool1", "tool2")

		assert.Equal(t, 0, registry.Size())
	})

	t.Run("unregister non-existing tool", func(t *testing.T) {
		registry := newToolRegistry()
		registry.Unregister("nonexistent")

		assert.Equal(t, 0, registry.Size())
	})

	t.Run("unregister with no names", func(t *testing.T) {
		registry := newToolRegistry()
		tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool)

		result := registry.Unregister()

		assert.Equal(t, registry, result)
		assert.Equal(t, 1, registry.Size())
	})

	t.Run("method chaining", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool1, tool2)

		result := registry.Unregister("tool1").Unregister("tool2")

		assert.Equal(t, registry, result)
		assert.Equal(t, 0, registry.Size())
	})
}

func TestToolRegistry_Find(t *testing.T) {
	t.Run("find existing tool", func(t *testing.T) {
		registry := newToolRegistry()
		tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool)

		found, exists := registry.Find("tool1")
		assert.True(t, exists)
		assert.NotNil(t, found)
		assert.Equal(t, "tool1", found.Definition().Name)
	})

	t.Run("find non-existing tool", func(t *testing.T) {
		registry := newToolRegistry()

		found, exists := registry.Find("nonexistent")
		assert.False(t, exists)
		assert.Nil(t, found)
	})
}

func TestToolRegistry_Exists(t *testing.T) {
	t.Run("tool exists", func(t *testing.T) {
		registry := newToolRegistry()
		tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool)

		assert.True(t, registry.Exists("tool1"))
	})

	t.Run("tool does not exist", func(t *testing.T) {
		registry := newToolRegistry()

		assert.False(t, registry.Exists("nonexistent"))
	})
}

func TestToolRegistry_All(t *testing.T) {
	t.Run("get all tools", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool1, tool2)

		all := registry.All()

		assert.Len(t, all, 2)
	})

	t.Run("empty registry", func(t *testing.T) {
		registry := newToolRegistry()

		all := registry.All()

		assert.Empty(t, all)
		assert.NotNil(t, all)
	})

	t.Run("modification safety", func(t *testing.T) {
		registry := newToolRegistry()
		tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool)

		all := registry.All()
		all[0] = nil

		assert.Equal(t, 1, registry.Size())
		found, _ := registry.Find("tool1")
		assert.NotNil(t, found)
	})
}

func TestToolRegistry_Names(t *testing.T) {
	t.Run("get all names", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool1, tool2)

		names := registry.Names()

		assert.Len(t, names, 2)
		assert.Contains(t, names, "tool1")
		assert.Contains(t, names, "tool2")
	})

	t.Run("empty registry", func(t *testing.T) {
		registry := newToolRegistry()

		names := registry.Names()

		assert.Empty(t, names)
		assert.NotNil(t, names)
	})
}

func TestToolRegistry_Size(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		registry := newToolRegistry()
		assert.Equal(t, 0, registry.Size())
	})

	t.Run("with tools", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool1, tool2)

		assert.Equal(t, 2, registry.Size())
	})
}

func TestToolRegistry_Clear(t *testing.T) {
	t.Run("clear registry", func(t *testing.T) {
		registry := newToolRegistry()
		tool1, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		tool2, _ := NewTool(ToolDefinition{Name: "tool2", InputSchema: "{}"}, ToolMetadata{}, nil)
		registry.Register(tool1, tool2)

		result := registry.Clear()

		assert.Equal(t, registry, result)
		assert.Equal(t, 0, registry.Size())
	})

	t.Run("clear empty registry", func(t *testing.T) {
		registry := newToolRegistry()

		result := registry.Clear()

		assert.Equal(t, registry, result)
		assert.Equal(t, 0, registry.Size())
	})
}

func TestToolRegistry_Concurrency(t *testing.T) {
	t.Run("concurrent register and find", func(t *testing.T) {
		registry := newToolRegistry()
		var wg sync.WaitGroup

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				tool, _ := NewTool(
					ToolDefinition{Name: "tool" + string(rune(idx)), InputSchema: "{}"},
					ToolMetadata{},
					nil,
				)
				registry.Register(tool)
			}(i)
		}

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				registry.Find("tool" + string(rune(idx)))
			}(i)
		}

		wg.Wait()
	})
}

func TestNewToolRegistry(t *testing.T) {
	t.Run("default capacity", func(t *testing.T) {
		registry := newToolRegistry()
		assert.NotNil(t, registry)
		assert.Equal(t, 0, registry.Size())
	})

	t.Run("with capacity hint", func(t *testing.T) {
		registry := newToolRegistry(10)
		assert.NotNil(t, registry)
		assert.Equal(t, 0, registry.Size())
	})

	t.Run("negative capacity", func(t *testing.T) {
		registry := newToolRegistry(-1)
		assert.NotNil(t, registry)
		assert.Equal(t, 0, registry.Size())
	})
}

func TestToolInvocationResult_ShouldContinue(t *testing.T) {
	t.Run("should continue when no external tools and not all return direct", func(t *testing.T) {
		result := &ToolInvocationResult{
			externalToolCalls: []*ToolCall{},
			allReturnDirect:   false,
		}

		assert.True(t, result.ShouldContinue())
	})

	t.Run("should not continue with external tools", func(t *testing.T) {
		result := &ToolInvocationResult{
			externalToolCalls: []*ToolCall{{ID: "1"}},
			allReturnDirect:   false,
		}

		assert.False(t, result.ShouldContinue())
	})

	t.Run("should not continue when all return direct", func(t *testing.T) {
		result := &ToolInvocationResult{
			externalToolCalls: []*ToolCall{},
			allReturnDirect:   true,
		}

		assert.False(t, result.ShouldContinue())
	})
}

func TestToolInvocationResult_ShouldReturn(t *testing.T) {
	t.Run("inverse of should continue", func(t *testing.T) {
		result := &ToolInvocationResult{
			externalToolCalls: []*ToolCall{},
			allReturnDirect:   false,
		}

		assert.Equal(t, !result.ShouldContinue(), result.ShouldReturn())
	})
}

func TestToolInvocationResult_BuildContinueRequest(t *testing.T) {
	t.Run("successful build", func(t *testing.T) {
		req, _ := NewRequest([]Message{NewUserMessage("test")})
		assistantMsg := NewAssistantMessage([]*ToolCall{{ID: "1", Name: "tool1"}})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		res, _ := NewResult(assistantMsg, metadata)
		resp, _ := NewResponse([]*Result{res}, &ResponseMetadata{})
		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "tool1", Result: "result"}})

		invResult := &ToolInvocationResult{
			request:         req,
			response:        resp,
			toolMessage:     toolMsg,
			allReturnDirect: false,
		}

		continueReq, err := invResult.BuildContinueRequest()
		require.NoError(t, err)
		assert.Len(t, continueReq.Messages, 3)
	})

	t.Run("error when should return", func(t *testing.T) {
		invResult := &ToolInvocationResult{
			allReturnDirect: true,
		}

		_, err := invResult.BuildContinueRequest()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "should return directly")
	})

	t.Run("error when request is nil", func(t *testing.T) {
		invResult := &ToolInvocationResult{
			allReturnDirect: false,
		}

		_, err := invResult.BuildContinueRequest()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "original chat request is required")
	})
}

func TestToolInvocationResult_BuildReturnResponse(t *testing.T) {
	t.Run("successful build", func(t *testing.T) {
		assistantMsg := NewAssistantMessage([]*ToolCall{{ID: "1", Name: "tool1"}})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		res, _ := NewResult(assistantMsg, metadata)
		resp, _ := NewResponse([]*Result{res}, &ResponseMetadata{})
		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "tool1", Result: "result"}})

		invResult := &ToolInvocationResult{
			response:          resp,
			toolMessage:       toolMsg,
			externalToolCalls: []*ToolCall{{ID: "1"}},
		}

		returnResp, err := invResult.BuildReturnResponse()
		require.NoError(t, err)
		assert.NotNil(t, returnResp)
	})

	t.Run("error when should continue", func(t *testing.T) {
		invResult := &ToolInvocationResult{
			allReturnDirect: false,
		}

		_, err := invResult.BuildReturnResponse()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "should continue with LLM")
	})
}

func TestNewToolSupport(t *testing.T) {
	t.Run("default capacity", func(t *testing.T) {
		support := NewToolSupport()
		assert.NotNil(t, support)
		assert.NotNil(t, support.Registry())
		assert.Equal(t, 0, support.Registry().Size())
	})

	t.Run("with capacity hint", func(t *testing.T) {
		support := NewToolSupport(10)
		assert.NotNil(t, support)
		assert.Equal(t, 0, support.Registry().Size())
	})
}

func TestToolSupport_RegisterTools(t *testing.T) {
	support := NewToolSupport()
	tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)

	support.RegisterTools(tool)

	assert.Equal(t, 1, support.Registry().Size())
}

func TestToolSupport_UnregisterTools(t *testing.T) {
	support := NewToolSupport()
	tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
	support.RegisterTools(tool)

	support.UnregisterTools("tool1")

	assert.Equal(t, 0, support.Registry().Size())
}

func TestToolSupport_ShouldReturnDirect(t *testing.T) {
	t.Run("returns true when all tools return direct", func(t *testing.T) {
		support := NewToolSupport()
		tool, _ := NewTool(
			ToolDefinition{Name: "tool1", InputSchema: "{}"},
			ToolMetadata{ReturnDirect: true},
			nil,
		)
		support.RegisterTools(tool)

		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "tool1", Result: "result"}})
		messages := []Message{toolMsg}

		assert.True(t, support.ShouldReturnDirect(messages))
	})

	t.Run("returns false when not all tools return direct", func(t *testing.T) {
		support := NewToolSupport()
		tool, _ := NewTool(
			ToolDefinition{Name: "tool1", InputSchema: "{}"},
			ToolMetadata{ReturnDirect: false},
			nil,
		)
		support.RegisterTools(tool)

		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "tool1", Result: "result"}})
		messages := []Message{toolMsg}

		assert.False(t, support.ShouldReturnDirect(messages))
	})

	t.Run("returns false when last message is not tool message", func(t *testing.T) {
		support := NewToolSupport()
		messages := []Message{NewUserMessage("test")}

		assert.False(t, support.ShouldReturnDirect(messages))
	})

	t.Run("returns false when tool not registered", func(t *testing.T) {
		support := NewToolSupport()
		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "unknown", Result: "result"}})
		messages := []Message{toolMsg}

		assert.False(t, support.ShouldReturnDirect(messages))
	})
}

func TestToolSupport_BuildReturnDirectResponse(t *testing.T) {
	t.Run("successful build", func(t *testing.T) {
		support := NewToolSupport()
		tool, _ := NewTool(
			ToolDefinition{Name: "tool1", InputSchema: "{}"},
			ToolMetadata{ReturnDirect: true},
			nil,
		)
		support.RegisterTools(tool)

		toolMsg, _ := NewToolMessage([]*ToolReturn{{ID: "1", Name: "tool1", Result: "result"}})
		messages := []Message{toolMsg}

		resp, err := support.BuildReturnDirectResponse(messages)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, FinishReasonReturnDirect, resp.Result().Metadata.FinishReason)
	})

	t.Run("error when conditions not met", func(t *testing.T) {
		support := NewToolSupport()
		messages := []Message{NewUserMessage("test")}

		_, err := support.BuildReturnDirectResponse(messages)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conditions not met")
	})
}

func TestToolSupport_ShouldInvokeToolCalls(t *testing.T) {
	t.Run("should invoke valid tool calls", func(t *testing.T) {
		support := NewToolSupport()
		tool, _ := NewTool(ToolDefinition{Name: "tool1", InputSchema: "{}"}, ToolMetadata{}, nil)
		support.RegisterTools(tool)

		assistantMsg := NewAssistantMessage([]*ToolCall{{ID: "1", Name: "tool1"}})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		res, _ := NewResult(assistantMsg, metadata)
		resp, _ := NewResponse([]*Result{res}, &ResponseMetadata{})

		shouldInvoke, err := support.ShouldInvokeToolCalls(resp)
		require.NoError(t, err)
		assert.True(t, shouldInvoke)
	})

	t.Run("error when tool not registered", func(t *testing.T) {
		support := NewToolSupport()

		assistantMsg := NewAssistantMessage([]*ToolCall{{ID: "1", Name: "unknown"}})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		res, _ := NewResult(assistantMsg, metadata)
		resp, _ := NewResponse([]*Result{res}, &ResponseMetadata{})

		shouldInvoke, err := support.ShouldInvokeToolCalls(resp)
		assert.Error(t, err)
		assert.False(t, shouldInvoke)
		assert.Contains(t, err.Error(), "tool not found")
	})
}

func TestToolSupport_InvokeToolCalls(t *testing.T) {
	t.Run("invoke internal tool", func(t *testing.T) {
		support := NewToolSupport()
		execFunc := func(ctx context.Context, args string) (string, error) {
			return "result", nil
		}
		tool, _ := NewTool(
			ToolDefinition{Name: "tool1", InputSchema: "{}"},
			ToolMetadata{ReturnDirect: false},
			execFunc,
		)
		support.RegisterTools(tool)

		req, _ := NewRequest([]Message{NewUserMessage("test")})
		assistantMsg := NewAssistantMessage([]*ToolCall{{ID: "1", Name: "tool1", Arguments: "{}"}})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		res, _ := NewResult(assistantMsg, metadata)
		resp, _ := NewResponse([]*Result{res}, &ResponseMetadata{})

		invResult, err := support.InvokeToolCalls(context.Background(), req, resp)
		require.NoError(t, err)
		assert.NotNil(t, invResult)
		assert.NotNil(t, invResult.toolMessage)
		assert.Len(t, invResult.toolMessage.ToolReturns, 1)
	})

	t.Run("invoke external tool", func(t *testing.T) {
		support := NewToolSupport()
		tool, _ := NewTool(
			ToolDefinition{Name: "tool1", InputSchema: "{}"},
			ToolMetadata{},
			nil,
		)
		support.RegisterTools(tool)

		req, _ := NewRequest([]Message{NewUserMessage("test")})
		assistantMsg := NewAssistantMessage([]*ToolCall{{ID: "1", Name: "tool1"}})
		metadata := &ResultMetadata{FinishReason: FinishReasonToolCalls}
		res, _ := NewResult(assistantMsg, metadata)
		resp, _ := NewResponse([]*Result{res}, &ResponseMetadata{})

		invResult, err := support.InvokeToolCalls(context.Background(), req, resp)
		require.NoError(t, err)
		assert.NotNil(t, invResult)
		assert.Len(t, invResult.externalToolCalls, 1)
	})
}
