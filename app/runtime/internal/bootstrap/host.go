package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Host owns the assembled application tier and its process-level close order
// (§13.2). The Stack is a pure discovery/delivery aggregate (§5.3); the Host holds
// the process resources, so delivery reaches coordinators through host.Stack while
// the composition root drives shutdown through Close.
type Host struct {
	Stack Stack

	// lifetime owns the immutable shutdown graph shared by every Host copy.
	lifetime *hostLifetime
}

// RecoverStartup completes durable work that must be reconciled before any
// delivery adapter starts accepting requests. Keeping it as a composition-root
// function, rather than a Host method, keeps Host's public surface limited to
// process lifetime ownership.
func RecoverStartup(ctx context.Context, stack Stack) error {
	if stack.Sessions == nil {
		return errors.New("runtime: sessions coordinator is unavailable for startup recovery")
	}
	return stack.Sessions.RecoverWorkspaceMutations(ctx)
}

type hostLifetime struct {
	closeMu         sync.Mutex
	stopping        bool
	closed          bool
	shutdownTimeout time.Duration

	goals        shutdownComponent
	integrations shutdownComponent
	codebase     shutdownComponent
	coordinator  shutdownComponent
	execution    shutdownComponent
	effectsTasks taskOwner
	toolClosers  []ShutdownResource
	resources    []ShutdownResource
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
		lifetime.goals,
		lifetime.integrations,
		lifetime.codebase,
		lifetime.coordinator,
	}
	if !lifetime.stopping {
		lifetime.stopping = true
		for _, component := range components {
			if component != nil {
				component.BeginShutdown()
			}
		}
		if lifetime.effectsTasks != nil {
			lifetime.effectsTasks.Cancel()
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
	if lifetime.effectsTasks != nil {
		errs = append(errs, lifetime.effectsTasks.Wait(shutdownCtx))
	}
	if componentErr := errors.Join(errs...); componentErr != nil {
		return componentErr
	}

	if lifetime.execution != nil {
		lifetime.execution.BeginShutdown()
		if err := lifetime.execution.AwaitShutdown(shutdownCtx); err != nil {
			return err
		}
	}
	var err error
	lifetime.toolClosers, err = closePendingResources(shutdownCtx, lifetime.toolClosers)
	if err != nil {
		// A closer that failed is still owned by this Host. Keep only those
		// unresolved steps so a later Close retries the real incomplete graph
		// without closing dependencies below an incomplete step.
		return err
	}
	lifetime.resources, err = closePendingResources(shutdownCtx, lifetime.resources)
	if err != nil {
		return err
	}
	lifetime.closed = true
	return nil
}
