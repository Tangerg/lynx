package runtimeembedded

import (
	"context"
	"encoding/json"
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

func (d *diagnosticToolAdapter) Tools(ctx context.Context) ([]diagnostictool.Descriptor, error) {
	r := d.runtime
	page, err := r.diagnosticTools.ListTools(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list diagnostic tools", page)
	if err != nil {
		return nil, err
	}
	tools := make([]diagnostictool.Descriptor, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		schema, marshalErr := json.Marshal(value.Parameters)
		if marshalErr != nil {
			return nil, runtimeContractViolation("list diagnostic tools item %d has an invalid schema: %v", index+1, marshalErr)
		}
		descriptor := diagnostictool.Descriptor{
			Name: value.Name, Description: value.Description,
			Schema: schema, Safety: diagnostictool.Safety(value.SafetyClass),
		}
		if err := descriptor.Validate(); err != nil {
			return nil, runtimeContractViolation("list diagnostic tools item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[descriptor.Name]; duplicate {
			return nil, runtimeContractViolation("list diagnostic tools repeats %q", descriptor.Name)
		}
		seen[descriptor.Name] = struct{}{}
		tools = append(tools, descriptor)
	}
	return tools, nil
}

func (d *diagnosticToolAdapter) Invoke(ctx context.Context, invocation diagnostictool.Invocation) (diagnostictool.Result, error) {
	r := d.runtime
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
		return diagnostictool.Result{}, runtimeContractViolation("diagnostic tool result cannot be encoded: %v", err)
	}
	result := diagnostictool.Result{JSON: encoded}
	if err := result.Validate(); err != nil {
		return diagnostictool.Result{}, runtimeContractViolation("diagnostic tool returned an invalid result: %v", err)
	}
	return result, nil
}
