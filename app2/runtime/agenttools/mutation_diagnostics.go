package agenttools

import (
	"context"
	"fmt"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/codeintel"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type mutationDiagnosticsTool struct {
	toolcontract.Tool
	paths     mutationPaths
	executor  *workspacefs.ConfinedExecutor
	codeIntel *codeintel.Service
}

func (tool *mutationDiagnosticsTool) Unwrap() toolcontract.Tool { return tool.Tool }

func (tool *mutationDiagnosticsTool) Call(ctx context.Context, arguments string) (string, error) {
	paths, err := tool.paths.MutationPaths(arguments)
	if err != nil {
		return "", fmt.Errorf("agenttools: inspect mutation paths for diagnostics: %w", err)
	}
	if len(paths) != 1 {
		return tool.Tool.Call(ctx, arguments)
	}
	path, err := tool.executor.AbsolutePath(paths[0])
	if err != nil {
		return "", err
	}
	return tool.codeIntel.DiagnoseMutation(ctx, tool.executor.Root(), path, func() (string, error) {
		return tool.Tool.Call(ctx, arguments)
	})
}
