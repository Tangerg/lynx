package runtimehost

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/capabilityflow"
)

type runtimeAgentDocuments struct {
	capabilities *capabilityflow.Service
}

func (source runtimeAgentDocuments) Documents(
	ctx context.Context,
	workspace string,
) ([]agentexec.AgentDocument, error) {
	documents, err := source.capabilities.AgentDocuments(ctx, workspace)
	if err != nil {
		return nil, err
	}
	values := make([]agentexec.AgentDocument, 0, len(documents))
	for _, document := range documents {
		values = append(values, agentexec.AgentDocument{
			Path: document.Path, Content: document.Content,
		})
	}
	return values, nil
}

var _ agentexec.AgentDocumentSource = runtimeAgentDocuments{}
