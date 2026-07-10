package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// wireMaintenancePorts fills the engine's compaction + extraction SPIs with the
// in-house maintenance services when the composition root didn't inject its own.
func wireMaintenancePorts(ecfg *kernel.Config, cfg lyraruntime.Config, historyStore history.Store, resolveUtility func(context.Context) *chat.Client) {
	if ecfg.Compactor == nil {
		// Window-relative compaction trigger: resolve the default turn model's
		// context window from the catalog so compaction fires relative to the
		// real model. Catalog miss leaves the compactor's fixed fallback.
		window := 0
		if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
			window = int(info.Limits.ContextWindow)
		}
		ecfg.Compactor = maintenance.NewCompactor(historyStore, resolveUtility, maintenance.CompactionConfig{ContextWindow: window})
	}
	if ecfg.Extractor == nil && cfg.Engine.Knowledge != nil {
		ecfg.Extractor = maintenance.NewExtractor(historyStore, cfg.Engine.Knowledge, resolveUtility)
	}
}
