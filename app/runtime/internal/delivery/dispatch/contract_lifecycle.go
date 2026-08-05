package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerLifecycle(r *Registry) {
	// runtime.discover takes no params; struct{} makes an unexpected field a
	// decode failure rather than something silently ignored.
	Query(r, MethodMeta{Name: "runtime.discover", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.DiscoverResponse, error) {
			return d.api.Discover(ctx)
		})
}
