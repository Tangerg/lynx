// Package toolflow exposes the deliberately small read-only diagnostic catalog
// that is safe to invoke outside an Agent Run.
package toolflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	toolcontract "github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools/fs"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type Resolver interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type Service struct {
	resolver Resolver
}

func New(resolver Resolver) (*Service, error) {
	if resolver == nil {
		return nil, errors.New("toolflow: resolver is required")
	}
	return &Service{resolver: resolver}, nil
}

func (service *Service) List(context.Context) (*protocol.Page[protocol.ToolSpec], error) {
	candidates := directTools(nil)
	values := make([]protocol.ToolSpec, 0, len(candidates))
	for _, candidate := range candidates {
		definition := candidate.Definition()
		var parameters map[string]any
		if err := json.Unmarshal(definition.InputSchema, &parameters); err != nil {
			return nil, fmt.Errorf("toolflow: decode %s schema: %w", definition.Name, err)
		}
		values = append(values, protocol.ToolSpec{
			Name: definition.Name, Description: definition.Description,
			Parameters: parameters, SafetyClass: protocol.SafetyClassSafe,
		})
	}
	return protocol.NewPage(values), nil
}

func (service *Service) Invoke(ctx context.Context, request protocol.InvokeToolRequest) (any, error) {
	if !isDirectTool(request.Name) {
		return nil, fmt.Errorf(
			"%w: direct tool %q is not registered",
			protocol.ErrInvalidParams,
			request.Name,
		)
	}
	if request.Workspace == nil {
		return nil, fmt.Errorf("%w: workspace is required", protocol.ErrInvalidParams)
	}
	resolved, err := service.resolver.Resolve(ctx, request.Workspace.Path)
	if err != nil || !resolved.Available {
		return nil, protocol.ErrWorkspaceUnavailable
	}
	executor, err := workspacefs.NewConfinedExecutor(resolved.Workspace.Path())
	if err != nil {
		return nil, err
	}
	arguments, err := json.Marshal(request.Arguments)
	if err != nil {
		return nil, fmt.Errorf("%w: arguments are invalid", protocol.ErrInvalidParams)
	}
	for _, candidate := range directTools(executor) {
		if candidate.Definition().Name != request.Name {
			continue
		}
		output, err := candidate.Call(ctx, string(arguments))
		if err != nil {
			return nil, err
		}
		return decodeResult(output), nil
	}
	return nil, errors.New("toolflow: validated direct tool disappeared")
}

func directTools(executor fs.Executor) []toolcontract.Tool {
	return []toolcontract.Tool{
		fs.NewReadTool(executor),
		fs.NewGlobTool(executor),
		fs.NewGrepTool(executor),
	}
}

func isDirectTool(name string) bool {
	for _, candidate := range directTools(nil) {
		if candidate.Definition().Name == name {
			return true
		}
	}
	return false
}

func decodeResult(output string) any {
	var value any
	if err := json.Unmarshal([]byte(output), &value); err == nil {
		return value
	}
	return output
}
