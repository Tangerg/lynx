package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
)

// Host owns the assembled application tier and its process-level close order
// (§13.2). Its application capsule exposes behavior only inside Bootstrap;
// process resources remain in the immutable shared shutdown graph.
type Host struct {
	application *hostApplication
	// lifetime owns the immutable shutdown graph shared by every Host copy.
	lifetime *hostLifetime
}

type hostLifetime struct {
	context         context.Context
	closeMu         sync.Mutex
	stopping        bool
	closed          bool
	shutdownTimeout time.Duration

	goalDriver          shutdownComponent
	mcpCoordinator      shutdownComponent
	codebaseCoordinator shutdownComponent
	runCoordinator      shutdownComponent
	executor            shutdownComponent
	runEffectTasks      taskOwner
	toolResources       []*teardown.Step
	hostResources       []*teardown.Step
	resourceGraph       *teardown.Sequence
}

type shutdownComponent interface {
	BeginShutdown()
	AwaitShutdown(ctx context.Context) error
}

type taskOwner interface {
	Cancel()
	Wait(ctx context.Context) error
}

const hostShutdownTimeout = 10 * time.Second

// Close shuts the assembled application tier down in reverse dependency order
// (§10.3). It first broadcasts cancellation to every task-owning component,
// then joins them under one Host-owned deadline. A component timeout leaves the
// graph in its stopping phase for a later Close budget. Once terminal resource
// teardown starts, its Sequence runs the whole reverse graph to completion past
// caller timeout; later Close calls only join that generation. Idempotent across
// Host copies once the graph has fully closed.
func (h *Host) Close() error {
	if h == nil || h.lifetime == nil {
		return nil
	}
	return closeHostLifetime(h.lifetime)
}

func closeHostLifetime(lifetime *hostLifetime) error {
	if lifetime == nil {
		return nil
	}
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	if lifetime.closed {
		return nil
	}
	components := []shutdownComponent{
		lifetime.goalDriver,
		lifetime.mcpCoordinator,
		lifetime.codebaseCoordinator,
		lifetime.runCoordinator,
	}
	if !lifetime.stopping {
		lifetime.stopping = true
		for _, component := range components {
			if component != nil {
				component.BeginShutdown()
			}
		}
		if lifetime.runEffectTasks != nil {
			lifetime.runEffectTasks.Cancel()
		}
	}

	timeout := lifetime.shutdownTimeout
	if timeout <= 0 {
		timeout = hostShutdownTimeout
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var errs []error
	for _, component := range components {
		if component != nil {
			errs = append(errs, component.AwaitShutdown(shutdownCtx))
		}
	}
	if lifetime.runEffectTasks != nil {
		errs = append(errs, lifetime.runEffectTasks.Wait(shutdownCtx))
	}
	if componentErr := errors.Join(errs...); componentErr != nil {
		return componentErr
	}

	if lifetime.executor != nil {
		lifetime.executor.BeginShutdown()
		if err := lifetime.executor.AwaitShutdown(shutdownCtx); err != nil {
			return err
		}
	}
	if lifetime.resourceGraph == nil {
		// host resources are acquired before tool resources; concatenating them
		// in creation order lets the Sequence own the whole reverse dependency
		// graph in one self-continuing generation.
		steps := make([]*teardown.Step, 0, len(lifetime.hostResources)+len(lifetime.toolResources))
		steps = append(steps, lifetime.hostResources...)
		steps = append(steps, lifetime.toolResources...)
		lifetime.resourceGraph = teardown.NewSequence(steps)
	}
	settled, resourceErr := lifetime.resourceGraph.Shutdown(shutdownCtx)
	if !settled {
		return resourceErr
	}
	lifetime.toolResources = nil
	lifetime.hostResources = nil
	lifetime.closed = true
	return resourceErr
}
