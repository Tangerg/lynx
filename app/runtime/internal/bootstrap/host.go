package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"
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
	toolResources       []ShutdownResource
	hostResources       []ShutdownResource
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
// then joins them under one Host-owned deadline. A timeout leaves the graph in
// its stopping phase: a later Close gets a new caller-owned shutdown budget and
// resumes the same in-flight teardown before closing dependent resources.
// Idempotent across Host copies once the graph has fully closed.
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
	var err error
	lifetime.toolResources, err = closePendingResources(shutdownCtx, lifetime.toolResources)
	if err != nil {
		// A closer that failed is still owned by this Host. Keep only those
		// unresolved steps so a later Close retries the real incomplete graph
		// without closing dependencies below an incomplete step.
		return err
	}
	lifetime.hostResources, err = closePendingResources(shutdownCtx, lifetime.hostResources)
	if err != nil {
		return err
	}
	lifetime.closed = true
	return nil
}
