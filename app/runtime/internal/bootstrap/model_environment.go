package bootstrap

import (
	"context"

	"github.com/Tangerg/scope/core/chatclient"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/modelclient"
	agentmemoryapp "github.com/Tangerg/scope/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/application/models"
)

// modelEnvironment is the composition-time graph shared by interactive model
// execution, utility-model work, and embedding-backed search. Its live role
// states let configuration changes take effect without rebuilding the Host.
type modelEnvironment struct {
	chatResolver       *modelclient.ChatResolver
	utilityRoleState   *models.RoleState
	utilityClient      func(context.Context) *chatclient.Client
	embeddingRoleState *models.RoleState
	embeddingResolver  *modelclient.EmbeddingResolver
	liveEmbedder       *modelclient.RoleEmbedder
	agentMemorySearch  *agentmemoryapp.Searcher
}

func buildModelEnvironment(ctx context.Context, cfg Config) (modelEnvironment, error) {
	chatResolver := modelclient.NewChatResolver(cfg.ProviderRegistry)
	utilityRole, err := loadUtilityRole(ctx, cfg.UtilityRoleStore)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)

	embeddingRole, err := loadEmbeddingRole(ctx, cfg.EmbeddingRoleStore)
	if err != nil {
		return modelEnvironment{}, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver := modelclient.NewEmbeddingResolver(cfg.ProviderRegistry)
	liveEmbedder := modelclient.NewRoleEmbedder(embeddingResolver, embeddingRoleState)

	environment := modelEnvironment{
		chatResolver:       chatResolver,
		utilityRoleState:   utilityRoleState,
		utilityClient:      chatResolver.UtilityClient(cfg.ChatClient, utilityRoleState),
		embeddingRoleState: embeddingRoleState,
		embeddingResolver:  embeddingResolver,
		liveEmbedder:       liveEmbedder,
	}
	if cfg.AgentMemoryStore != nil {
		environment.agentMemorySearch = agentmemoryapp.NewSearcher(cfg.AgentMemoryStore, liveEmbedder.ResolveMemory)
	}
	return environment, nil
}
