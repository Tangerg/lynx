package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	modelapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// ListModels projects the application-owned model-discovery result onto the
// protocol page. Discovery policy, remote fallback, and catalog enrichment all
// remain in application/models.
func (s *Server) ListModels(ctx context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	models := s.models.ListModels(ctx, in.Provider)
	out := make([]protocol.Model, 0, len(models))
	for _, model := range models {
		out = append(out, modelToWire(model))
	}
	return protocol.NewPage(out), nil
}

// GetUtilityRole reports the (provider, model) the in-house maintenance
// services run on — empty model when unset, meaning they run on the main turn
// model (models.getUtilityRole).
func (s *Server) GetUtilityRole(_ context.Context) (*protocol.UtilityRole, error) {
	role := s.models.UtilityRole()
	return &protocol.UtilityRole{Provider: role.Provider, Model: role.Model}, nil
}

// SetUtilityRole points the maintenance services at a (provider, model),
// validated and persisted by the application use case. Returns the stored role.
func (s *Server) SetUtilityRole(ctx context.Context, in protocol.UtilityRole) (*protocol.UtilityRole, error) {
	role, err := s.models.SetUtilityRole(ctx, in.Provider, in.Model)
	if err != nil {
		return nil, mapModelError(err)
	}
	return &protocol.UtilityRole{Provider: role.Provider, Model: role.Model}, nil
}

// GetEmbeddingRole reports the (provider, model) the @codebase semantic index
// embeds with — empty model when unset (the feature is off)
// (models.getEmbeddingRole).
func (s *Server) GetEmbeddingRole(_ context.Context) (*protocol.EmbeddingRole, error) {
	role := s.models.EmbeddingRole()
	return &protocol.EmbeddingRole{Provider: role.Provider, Model: role.Model}, nil
}

// SetEmbeddingRole points the index at an (embedding-capable provider, model),
// validated and persisted by the application use case. Returns the stored role.
func (s *Server) SetEmbeddingRole(ctx context.Context, in protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
	role, err := s.models.SetEmbeddingRole(ctx, in.Provider, in.Model)
	if err != nil {
		return nil, mapModelError(err)
	}
	return &protocol.EmbeddingRole{Provider: role.Provider, Model: role.Model}, nil
}

func mapModelError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, modelapp.ErrProviderUnsupported) ||
		errors.Is(err, modelapp.ErrProviderBaseURLRequired) ||
		errors.Is(err, modelapp.ErrProviderUnconfigured) ||
		errors.Is(err, modelapp.ErrEmbeddingUnsupported) ||
		errors.Is(err, modelrole.ErrProviderRequired) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}

func modelToWire(model modelapp.Model) protocol.Model {
	if model.Details == nil {
		return protocol.Model{ID: model.ID, Provider: model.Provider}
	}
	details := model.Details
	out := protocol.Model{
		ID:              model.ID,
		Provider:        model.Provider,
		DisplayName:     details.DisplayName,
		ContextWindow:   details.ContextWindow,
		MaxInputTokens:  details.MaxInputTokens,
		MaxOutputTokens: details.MaxOutputTokens,
		Deprecated:      details.Deprecated,
		Capabilities: &protocol.ModelCapabilities{
			Reasoning:             details.Reasoning,
			ReasoningLevels:       details.ReasoningLevels,
			ReasoningDefaultLevel: details.ReasoningDefault,
			Multimodal:            details.Multimodal,
			InputModalities:       toWireModalities(details.InputModalities),
			OutputModalities:      toWireModalities(details.OutputModalities),
			ToolUse:               details.ToolUse,
			StructuredOutput:      details.StructuredOutput,
		},
	}
	if !details.KnowledgeCutoff.IsZero() {
		out.KnowledgeCutoff = details.KnowledgeCutoff.Format(time.DateOnly)
	}
	if details.Pricing != nil {
		out.Pricing = &protocol.ModelPricing{
			InputUsdPerMillionTokens:      details.Pricing.InputPerMillion,
			OutputUsdPerMillionTokens:     details.Pricing.OutputPerMillion,
			CacheReadUsdPerMillionTokens:  details.Pricing.CacheReadPerMillion,
			CacheWriteUsdPerMillionTokens: details.Pricing.CacheWritePerMillion,
		}
	}
	return out
}

func toWireModalities(in []string) []protocol.Modality {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.Modality, len(in))
	for i, modality := range in {
		out[i] = protocol.Modality(modality)
	}
	return out
}
