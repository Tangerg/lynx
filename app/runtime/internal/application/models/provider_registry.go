package models

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/domain/provider"
)

// ProviderRegistry is the exact credential-registry capability consumed by
// model configuration use cases.
type ProviderRegistry interface {
	List(ctx context.Context) ([]provider.Provider, error)
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
	Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error)
}
