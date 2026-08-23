package runtimehost

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/capabilityflow"
)

type runtimeKnowledgeDocuments struct {
	capabilities *capabilityflow.Service
}

func (source runtimeKnowledgeDocuments) Knowledge(
	ctx context.Context,
	workspace string,
) ([]agentexec.KnowledgeDocument, error) {
	documents, err := source.capabilities.KnowledgeDocuments(ctx, workspace)
	if err != nil {
		return nil, err
	}
	values := make([]agentexec.KnowledgeDocument, 0, len(documents))
	for _, document := range documents {
		values = append(values, agentexec.KnowledgeDocument{
			Path: document.Path, Content: document.Content,
		})
	}
	return values, nil
}

var _ agentexec.KnowledgeDocumentSource = runtimeKnowledgeDocuments{}
