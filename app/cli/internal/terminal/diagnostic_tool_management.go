package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
)

func (a *app) ShowDiagnosticTools() {
	if a.diagnosticTools == nil {
		a.message("this runtime composition has no diagnostic tool service")
		return
	}
	a.runRuntimeReaderQuery("loading diagnostic tools", runtimeReaderDiagnosticTools,
		func(ctx context.Context) (readerDocument, error) {
			tools, err := a.diagnosticTools.Tools(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return diagnosticToolsDocument(tools), nil
		})
}

func diagnosticToolsDocument(tools []diagnostictool.Descriptor) readerDocument {
	if len(tools) == 0 {
		return paragraphDocument("Diagnostic tools", "none available", []string{"No direct read-only diagnostics are available."})
	}
	sections := make([]ToolSection, 0, len(tools)*2)
	for _, tool := range tools {
		description := strings.TrimSpace(tool.Description)
		if description == "" {
			description = "No description provided."
		}
		sections = append(sections,
			ToolSection{Title: tool.Name + " · " + string(tool.Safety), Style: toolSectionParagraph, Text: description},
			ToolSection{Title: "Arguments", Style: toolSectionCode, Language: "json", Text: prettyJSON(tool.Schema)},
		)
	}
	return readerDocument{Title: "Diagnostic tools", Detail: fmt.Sprintf("%d read-only tools", len(tools)), Sections: sections}
}

func (a *app) InvokeDiagnosticTool(argument string) error {
	if a.diagnosticTools == nil {
		return errors.New("this runtime composition has no diagnostic tool service")
	}
	identity, arguments, err := parseDiagnosticToolInvocation(argument)
	if err != nil {
		return err
	}
	workspace := a.session.Workspace.Path
	a.status.note("invoking diagnostic tool " + identity)
	started := a.runOperation(diagnosticToolOperation, false,
		func(ctx context.Context) (diagnosticInvocationResult, error) {
			tools, err := a.diagnosticTools.Tools(ctx)
			if err != nil {
				return diagnosticInvocationResult{}, err
			}
			tool, err := resolveDiagnosticTool(tools, identity)
			if err != nil {
				return diagnosticInvocationResult{}, err
			}
			result, err := a.diagnosticTools.Invoke(ctx, diagnostictool.Invocation{
				Tool: tool, Arguments: arguments, Workspace: workspace,
			})
			return diagnosticInvocationResult{tool: tool, result: result}, err
		},
		func(invocation diagnosticInvocationResult, err error) {
			if err != nil {
				a.message("invoke diagnostic tool failed: " + err.Error())
				return
			}
			a.setRuntimeReader(runtimeReaderDiagnosticTools)
			a.workspaceReader = workspaceReaderNone
			a.openReaderDocument(readerDocument{
				Title: "Diagnostic · " + invocation.tool.Name, Detail: workspace,
				Sections: []ToolSection{{Title: "Result", Style: toolSectionCode, Language: "json", Text: prettyJSON(invocation.result.JSON)}},
			})
			a.status.note("diagnostic complete · " + invocation.tool.Name)
		},
	)
	if !started {
		return errors.New("another diagnostic tool operation is running")
	}
	return nil
}

type diagnosticInvocationResult struct {
	tool   diagnostictool.Descriptor
	result diagnostictool.Result
}

func parseDiagnosticToolInvocation(argument string) (string, json.RawMessage, error) {
	identity, raw, ok := splitCommandArgument(argument)
	if !ok {
		return "", nil, errors.New("usage: /tool-invoke <name> [json-object]")
	}
	arguments, err := diagnostictool.ParseArguments(raw)
	if err != nil {
		return "", nil, fmt.Errorf("usage: /tool-invoke <name> [json-object]: %w", err)
	}
	return identity, arguments, nil
}

func resolveDiagnosticTool(tools []diagnostictool.Descriptor, identity string) (diagnostictool.Descriptor, error) {
	for _, tool := range tools {
		if tool.Name == identity {
			return tool.Clone(), nil
		}
	}
	var matches []diagnostictool.Descriptor
	for _, tool := range tools {
		if strings.HasPrefix(tool.Name, identity) {
			matches = append(matches, tool)
		}
	}
	switch len(matches) {
	case 0:
		return diagnostictool.Descriptor{}, errors.New("diagnostic tool not found: " + identity)
	case 1:
		return matches[0].Clone(), nil
	default:
		return diagnostictool.Descriptor{}, errors.New("diagnostic tool name is ambiguous; use the full name")
	}
}
