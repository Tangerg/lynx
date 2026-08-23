package runtimehost

import (
	"context"

	"github.com/Tangerg/lynx/core/embedding"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/agenttools"
	"github.com/Tangerg/lynx/app2/runtime/memoryflow"
	"github.com/Tangerg/lynx/app2/runtime/providerflow"
)

type runtimeMemory struct {
	service *memoryflow.Service
}

func (source runtimeMemory) Recall(
	ctx context.Context,
	workspace string,
) ([]agentexec.MemoryItem, error) {
	items, err := source.service.Effective(ctx, workspace)
	if err != nil {
		return nil, err
	}
	values := make([]agentexec.MemoryItem, len(items))
	for index, item := range items {
		values[index] = agentexec.MemoryItem{
			ID: item.ID, Scope: string(item.Scope), Content: item.Content,
		}
	}
	return values, nil
}

func (source runtimeMemory) SearchMemory(
	ctx context.Context,
	workspace string,
	query string,
	limit int,
) ([]agenttools.MemoryHit, error) {
	items, err := source.service.Search(ctx, workspace, query, limit)
	if err != nil {
		return nil, err
	}
	values := make([]agenttools.MemoryHit, len(items))
	for index, item := range items {
		values[index] = agenttools.MemoryHit{
			Scope: string(item.Scope), Content: item.Content,
		}
	}
	return values, nil
}

type runtimeMemoryEmbedding struct {
	providers *providerflow.Service
}

func (models runtimeMemoryEmbedding) ResolveMemoryEmbedding(
	ctx context.Context,
) (embedding.Model, error) {
	model, _, err := models.providers.ResolveEmbedding(ctx)
	return model, err
}

var (
	_ agentexec.MemorySource     = runtimeMemory{}
	_ agenttools.MemoryGateway   = runtimeMemory{}
	_ memoryflow.EmbeddingModels = runtimeMemoryEmbedding{}
)
