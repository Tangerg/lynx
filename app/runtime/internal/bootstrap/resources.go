package bootstrap

import (
	"context"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/component/shutdown"
)

func shutdownResources(resources []ShutdownResource) []ShutdownResource {
	steps := make([]ShutdownResource, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		steps = append(steps, shutdown.New(resource.Shutdown))
	}
	return steps
}

func shutdownClosers(closers []func() error) []ShutdownResource {
	steps := make([]ShutdownResource, 0, len(closers))
	for _, closeFn := range closers {
		if closeFn == nil {
			continue
		}
		steps = append(steps, shutdown.New(func(context.Context) error {
			return closeFn()
		}))
	}
	return steps
}

func closePendingResources(ctx context.Context, resources []ShutdownResource) ([]ShutdownResource, error) {
	for index, resource := range slices.Backward(resources) {
		if resource != nil {
			if err := resource.Shutdown(ctx); err != nil {
				// The slice is creation ordered, so the not-yet-run prefix contains
				// dependencies of this failing closer. Do not tear them down beneath
				// an in-flight or failed dependent operation; retain that exact prefix
				// for a later Close instead.
				return slices.Clone(resources[:index+1]), err
			}
		}
	}
	return nil, nil
}
