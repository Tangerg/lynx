package bootstrap

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
)

func terminalResources(resources []TerminalResource) []*teardown.Step {
	steps := make([]*teardown.Step, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		steps = append(steps, teardown.Terminal(func(context.Context) error {
			return resource.Close()
		}))
	}
	return steps
}

func terminalClosers(closers []func() error) []*teardown.Step {
	steps := make([]*teardown.Step, 0, len(closers))
	for _, closeFn := range closers {
		if closeFn == nil {
			continue
		}
		steps = append(steps, teardown.Terminal(func(context.Context) error {
			return closeFn()
		}))
	}
	return steps
}

func closePendingResources(ctx context.Context, resources []*teardown.Step) ([]*teardown.Step, error) {
	var diagnostics []error
	for index, resource := range slices.Backward(resources) {
		if resource != nil {
			settled, err := resource.Shutdown(ctx)
			diagnostics = append(diagnostics, err)
			if !settled {
				// The slice is creation ordered, so the not-yet-run prefix contains
				// dependencies of this unfinished closer. Do not tear them down
				// beneath an in-flight or retryable dependent operation; retain that
				// exact prefix for a later Close instead. A settled terminal closer's
				// diagnostic does not block dependency teardown.
				return slices.Clone(resources[:index+1]), errors.Join(diagnostics...)
			}
		}
	}
	return nil, errors.Join(diagnostics...)
}
