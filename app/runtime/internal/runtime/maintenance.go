package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

func wireMaintenancePorts(ecfg *kernel.Config, cfg Config, memStore memory.Store, resolveUtility func(context.Context) *chat.Client) {
	if ecfg.Compactor == nil {
		// Window-relative compaction trigger: resolve the default turn model's
		// context window from the catalog so compaction fires relative to the
		// real model. Catalog miss leaves the compactor's fixed fallback.
		window := 0
		if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
			window = int(info.Limits.ContextWindow)
		}
		ecfg.Compactor = maintenance.NewCompactor(memStore, resolveUtility, maintenance.CompactionConfig{ContextWindow: window})
	}
	if ecfg.Extractor == nil && cfg.Engine.Knowledge != nil {
		ecfg.Extractor = maintenance.NewExtractor(memStore, cfg.Engine.Knowledge, resolveUtility)
	}
}
