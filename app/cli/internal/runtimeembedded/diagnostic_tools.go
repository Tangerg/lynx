package runtimeembedded

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
)

type diagnosticToolBinding interface {
	ListTools(context.Context, embedded.CallOptions) (*protocol.Page[protocol.ToolSpec], error)
	InvokeTool(context.Context, protocol.InvokeToolRequest, embedded.CommandOptions) (any, error)
}

type diagnosticToolAdapter struct{ runtime *Runtime }

var _ diagnostictool.Service = (*diagnosticToolAdapter)(nil)

func (adapter *diagnosticToolAdapter) Tools(ctx context.Context) ([]diagnostictool.Descriptor, error) {
	r := adapter.runtime
	page, err := r.diagnosticTools.ListTools(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if page == nil {
		return nil, errors.New("list diagnostic tools: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list diagnostic tools: runtime returned an unusable continuation cursor")
	}
	tools := make([]diagnostictool.Descriptor, 0, len(page.Data))
	seen := make(map[string]struct{}, len(page.Data))
	for index, value := range page.Data {
		schema, marshalErr := json.Marshal(value.Parameters)
		if marshalErr != nil {
			return nil, fmt.Errorf("list diagnostic tools item %d schema: %w", index+1, marshalErr)
		}
		descriptor := diagnostictool.Descriptor{
			Name: value.Name, Description: value.Description,
			Schema: schema, Safety: diagnostictool.Safety(value.SafetyClass),
		}
		if err := descriptor.Validate(); err != nil {
			return nil, fmt.Errorf("list diagnostic tools item %d: %w", index+1, err)
		}
		if _, duplicate := seen[descriptor.Name]; duplicate {
			return nil, fmt.Errorf("list diagnostic tools repeats %q", descriptor.Name)
		}
		seen[descriptor.Name] = struct{}{}
		tools = append(tools, descriptor)
	}
	return tools, nil
}

func (adapter *diagnosticToolAdapter) Invoke(ctx context.Context, invocation diagnostictool.Invocation) (diagnostictool.Result, error) {
	r := adapter.runtime
	if err := invocation.Validate(); err != nil {
		return diagnostictool.Result{}, err
	}
	var arguments map[string]any
	if err := json.Unmarshal(invocation.Arguments, &arguments); err != nil {
		return diagnostictool.Result{}, fmt.Errorf("decode diagnostic tool arguments: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return diagnostictool.Result{}, err
	}
	value, err := r.diagnosticTools.InvokeTool(ctx, protocol.InvokeToolRequest{
		Name: strings.TrimSpace(invocation.Tool.Name), Arguments: arguments,
		Workspace: &protocol.WorkspaceRef{Path: strings.TrimSpace(invocation.Workspace)},
	}, options)
	if err != nil {
		return diagnostictool.Result{}, classifyError(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return diagnostictool.Result{}, fmt.Errorf("encode diagnostic tool result: %w", err)
	}
	result := diagnostictool.Result{JSON: encoded}
	if err := result.Validate(); err != nil {
		return diagnostictool.Result{}, fmt.Errorf("invoke diagnostic tool: %w", err)
	}
	return result, nil
}
