package bootstrap

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/infra/teardown"
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
