package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

func (p *Process) requestChildAdmission(ctx context.Context) error {
	admitter, ok := firstExtension[ChildAdmitter](p.combinedExtensionsResolverFirst())
	if !ok {
		return nil
	}
	if err := admitChildWith(ctx, admitter, p); err != nil {
		return fmt.Errorf("child admitter %q: %w", admitter.name, err)
	}
	return nil
}

func admitChildWith(
	ctx context.Context,
	admitter extensionCapability[ChildAdmitter],
	child core.ProcessView,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("child admitter %q panicked", admitter.name), recovered)
		}
	}()
	return admitter.value.AdmitChild(normalizeContext(ctx), child)
}

// discardRejectedChild removes a child that never crossed ProcessCreated. The
// process-tree mutation excludes Kill/Snapshot/Remove while the unpublished
// node is terminalized and detached from the registry and budget tree.
func (e *Engine) discardRejectedChild(ctx context.Context, parent, child *Process, cause error) error {
	if e == nil || parent == nil || child == nil {
		return nil
	}
	cleanupCtx := context.WithoutCancel(normalizeContext(ctx))
	releaseMutation, err := e.processMutations.acquire(cleanupCtx, e.processTreeRootID(parent))
	if err != nil {
		return fmt.Errorf("runtime: discard rejected child %q: acquire process tree: %w", child.ID(), err)
	}
	defer releaseMutation()

	// Another tree owner may have removed the complete tree after admission
	// failed. In that case it already released the child's deployment.
	if !e.processes.available(child) {
		return nil
	}
	child.state.markKilled(cause)
	_, _ = child.state.endRun()
	if !e.processes.reserveProcesses([]*Process{child}) {
		return fmt.Errorf("runtime: discard rejected child %q: reserve removal", child.ID())
	}
	if !child.state.removable() {
		e.processes.releaseProcesses([]*Process{child})
		return fmt.Errorf("runtime: discard rejected child %q: process remained active", child.ID())
	}
	if !e.processes.unregisterReservedTree([]*Process{child}) {
		e.processes.releaseProcesses([]*Process{child})
		return fmt.Errorf("runtime: discard rejected child %q: unregister process", child.ID())
	}
	detached := parent.budget.removeChild(child)
	child.releaseDeployment()
	if !detached {
		return fmt.Errorf("runtime: discard rejected child %q: detach budget ownership", child.ID())
	}
	return nil
}
