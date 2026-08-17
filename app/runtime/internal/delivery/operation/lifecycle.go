package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerLifecycle(registry *Registry) {
	// runtime.discover takes no params; struct{} makes an unexpected field a
	// decode failure rather than something silently ignored.
	Query(registry, MethodMeta{Name: "runtime.discover", Stability: stable},
		func(service interface {
			Discover(context.Context) (*protocol.DiscoverResponse, error)
		}, ctx context.Context, _ struct{}) (*protocol.DiscoverResponse, error) {
			return service.Discover(ctx)
		})
}
