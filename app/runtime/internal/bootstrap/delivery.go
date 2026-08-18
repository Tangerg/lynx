package bootstrap

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/ownershiprecovery"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// hostApplication is the immutable application capsule owned by Host. Delivery
// receives its own consumer config; startup recovery and workers stay behind
// behavior methods instead of leaking a coordinator locator to Instance.
type hostApplication struct {
	delivery         server.Config
	sessions         *sessions.Coordinator
	workers          hostWorkers
	idempotencyStore idempotency.Store
}

type hostWorkers struct {
	scheduler     *schedules.Firing
	recovery      *ownershiprecovery.Coordinator
	invalidations invalidation.Publish
}

type workerJoins struct {
	scheduler <-chan struct{}
	recovery  <-chan struct{}
}

type operationDelivery struct {
	endpoint *operation.Endpoint
	service  *server.Server
}

const ownershipRecoveryInterval = time.Second

func (application *hostApplication) recoverStartup(ctx context.Context) error {
	return application.sessions.RecoverWorkspaceMutations(ctx)
}

func (application *hostApplication) newOperationService(
	info protocol.ServerInfo,
	idempotencyNamespace string,
) (*server.Server, error) {
	cfg := application.delivery
	cfg.ServerInfo = info
	cfg.IdempotencyLimits = protocol.IdempotencyLimits{
		RetentionSeconds: int(idempotency.Retention.Seconds()),
		Namespace:        idempotencyNamespace,
	}
	return server.New(cfg)
}

func (application *hostApplication) openOperationDelivery(
	lifetime context.Context,
	info protocol.ServerInfo,
	idempotencyNamespace string,
) (operationDelivery, error) {
	service, err := application.newOperationService(info, idempotencyNamespace)
	if err != nil {
		return operationDelivery{}, err
	}
	endpoint, err := operation.New(service, operation.Config{
		IdempotencyStore:     application.idempotencyStore,
		IdempotencyNamespace: idempotencyNamespace,
		Lifetime:             lifetime,
	})
	if err != nil {
		service.Close()
		return operationDelivery{}, err
	}
	return operationDelivery{endpoint: endpoint, service: service}, nil
}

func (delivery operationDelivery) beginShutdown() {
	if delivery.service != nil {
		delivery.service.Close()
	}
	if delivery.endpoint != nil {
		delivery.endpoint.BeginShutdown()
	}
}

func (delivery operationDelivery) awaitShutdown(ctx context.Context) error {
	if delivery.endpoint == nil {
		return nil
	}
	return delivery.endpoint.AwaitShutdown(ctx)
}

func (application *hostApplication) notifyExternalChange() {
	application.workers.invalidations.Notify(invalidation.Notice{Resource: invalidation.Resync})
}

func (application *hostApplication) startWorkers(ctx context.Context) workerJoins {
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		application.workers.scheduler.RunWorker(ctx)
	}()
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		application.workers.runOwnershipRecovery(ctx)
	}()
	return workerJoins{scheduler: schedulerDone, recovery: recoveryDone}
}

// runOwnershipRecovery detects process death by attempting the same kernel
// leases held by live Run and Goal owners. A contended lease is definitive
// liveness evidence; no heartbeat or expiry clock participates.
func (workers hostWorkers) runOwnershipRecovery(ctx context.Context) {
	ticker := time.NewTicker(ownershipRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = workers.recovery.Reconcile(ctx)
		}
	}
}
