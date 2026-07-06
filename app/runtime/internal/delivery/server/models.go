package server

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListModels enumerates the models a provider offers, from the embedded
// catalog with full metadata (context window, capabilities, pricing). Served
// straight from the static catalog — no key required (API.md §7.6).
func (s *Server) ListModels(_ context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	models := catalog.Models(in.Provider)
	out := make([]protocol.Model, 0, len(models))
	for _, m := range models {
		out = append(out, modelToWire(in.Provider, m))
	}
	return protocol.NewPage(out), nil
}

// GetUtilityRole reports the (provider, model) the in-house maintenance
// services run on — empty model when unset, meaning they run on the main turn
// model (models.getUtilityRole).
func (s *Server) GetUtilityRole(_ context.Context) (*protocol.UtilityRole, error) {
	p, m := s.modelRoles.UtilityRole()
	return &protocol.UtilityRole{Provider: p, Model: m}, nil
}

// SetUtilityRole points the maintenance services at a (provider, model),
// validated by building its client; an empty model clears the role back to the
// main turn model (models.setUtilityRole). Returns the stored role.
func (s *Server) SetUtilityRole(ctx context.Context, in protocol.UtilityRole) (*protocol.UtilityRole, error) {
	if err := s.validateUtilityRole(ctx, in); err != nil {
		return nil, err
	}
	if err := s.modelRoles.SetUtilityRole(ctx, in.Provider, in.Model); err != nil {
		return nil, err
	}
	p, m := s.modelRoles.UtilityRole()
	return &protocol.UtilityRole{Provider: p, Model: m}, nil
}

// GetEmbeddingRole reports the (provider, model) the @codebase semantic index
// embeds with — empty model when unset (the feature is off)
// (models.getEmbeddingRole).
func (s *Server) GetEmbeddingRole(_ context.Context) (*protocol.EmbeddingRole, error) {
	p, m := s.modelRoles.EmbeddingRole()
	return &protocol.EmbeddingRole{Provider: p, Model: m}, nil
}

// SetEmbeddingRole points the index at an (embedding-capable provider, model);
// an empty model clears it (models.setEmbeddingRole). The user-correctable
// preconditions are checked here as invalid_params; the runtime then builds the
// client + persists the role (a failure there is internal_error).
func (s *Server) SetEmbeddingRole(ctx context.Context, in protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
	if err := s.validateEmbeddingRole(ctx, in); err != nil {
		return nil, err
	}
	if err := s.modelRoles.SetEmbeddingRole(ctx, in.Provider, in.Model); err != nil {
		return nil, err
	}
	p, m := s.modelRoles.EmbeddingRole()
	return &protocol.EmbeddingRole{Provider: p, Model: m}, nil
}

func (s *Server) validateUtilityRole(ctx context.Context, in protocol.UtilityRole) error {
	if in.Model == "" {
		return nil
	}
	if _, ok := s.providers.ProviderMetadata(in.Provider); !ok {
		return fmt.Errorf("%w: provider %q is not supported", protocol.ErrInvalidParams, in.Provider)
	}
	return s.requireConfiguredProvider(ctx, in.Provider)
}

func (s *Server) validateEmbeddingRole(ctx context.Context, in protocol.EmbeddingRole) error {
	if in.Model == "" {
		return nil
	}
	meta, ok := s.providers.ProviderMetadata(in.Provider)
	if !ok || !meta.EmbeddingCapable {
		return fmt.Errorf("%w: provider %q has no embeddings adapter", protocol.ErrInvalidParams, in.Provider)
	}
	return s.requireConfiguredProvider(ctx, in.Provider)
}

func (s *Server) requireConfiguredProvider(ctx context.Context, providerID string) error {
	entry, ok, err := s.providers.GetRegisteredProvider(ctx, providerID)
	if err != nil {
		return err
	}
	if !ok || !entry.Enabled() {
		return fmt.Errorf("%w: provider %q is not configured (set its API key first)", protocol.ErrInvalidParams, providerID)
	}
	return nil
}

func modelToWire(providerID string, m chat.ModelInfo) protocol.Model {
	out := protocol.Model{
		ID:              m.ID,
		Provider:        providerID,
		DisplayName:     m.DisplayName,
		ContextWindow:   int(m.Limits.ContextWindow),
		MaxInputTokens:  int(m.Limits.MaxInputTokens),
		MaxOutputTokens: int(m.Limits.MaxOutputTokens),
		Deprecated:      m.Deprecated,
		Capabilities: &protocol.ModelCapabilities{
			Reasoning:             m.Reasoning.Supported,
			ReasoningLevels:       m.Reasoning.Levels,
			ReasoningDefaultLevel: m.Reasoning.DefaultLevel,
			Multimodal:            m.Modalities.AcceptsInput(chat.ModalityImage),
			InputModalities:       toWireModalities(m.Modalities.Input),
			OutputModalities:      toWireModalities(m.Modalities.Output),
			ToolUse:               m.ToolCall,
			StructuredOutput:      m.StructuredOutput,
		},
	}
	if !m.KnowledgeCutoff.IsZero() {
		out.KnowledgeCutoff = m.KnowledgeCutoff.Format(time.DateOnly)
	}
	if len(m.Pricing) > 0 {
		p := m.Pricing[0]
		out.Pricing = &protocol.ModelPricing{
			InputUsdPerMillionTokens:      p.InputPer1M,
			OutputUsdPerMillionTokens:     p.OutputPer1M,
			CacheReadUsdPerMillionTokens:  p.CacheReadPer1M,
			CacheWriteUsdPerMillionTokens: p.CacheWritePer1M,
		}
	}
	return out
}

func toWireModalities(in []chat.Modality) []protocol.Modality {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.Modality, len(in))
	for i, m := range in {
		out[i] = protocol.Modality(m)
	}
	return out
}
