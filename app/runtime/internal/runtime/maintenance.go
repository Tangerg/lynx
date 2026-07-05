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

// GenerateTitle derives a short session title from a conversation's opening
// user message — auto-naming an untitled session (the wire Session.title).
// Best-effort: returns "" (no error) when titling isn't possible. Lives here,
// like [Runtime.ProbeProvider], because the runtime owns the maintenance LLM
// client; the delivery layer triggers it off a finished root run.
func (r *Runtime) GenerateTitle(ctx context.Context, firstMessage string) (string, error) {
	return r.titler.Generate(ctx, firstMessage)
}
