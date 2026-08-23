// Package codebaseflow owns the semantic source-index use cases and the exact
// lifecycle of asynchronous rebuilds.
package codebaseflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/core/embedding"

	"github.com/Tangerg/lynx/app2/runtime/domain/codebase"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const (
	defaultSearchLimit = 8
	maxSearchLimit     = 50
	settlementTimeout  = 10 * time.Second
)

type Store interface {
	GetCodebaseIndex(context.Context, string) (codebase.Index, error)
	BeginCodebaseIndex(context.Context, codebase.Index) error
	CompleteCodebaseIndex(
		context.Context,
		string,
		codebase.Index,
		[]codebase.Document,
	) (bool, error)
	FailCodebaseIndex(context.Context, string, codebase.Index) (bool, error)
	ListCodebaseDocuments(context.Context, string) ([]codebase.Document, error)
}

type Resolver interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type Models interface {
	ResolveEmbedding(context.Context) (embedding.Model, protocol.EmbeddingRole, error)
}

type IDs interface {
	New(string) (string, error)
}

type Publisher interface {
	Publish(protocol.RuntimeEvent)
}

type workspaceOwner struct {
	admission   sync.Mutex
	generation  uint64
	operationID string
	cancel      context.CancelFunc
}

type Service struct {
	store    Store
	resolver Resolver
	models   Models
	ids      IDs
	events   Publisher
	lifetime context.Context
	logger   *slog.Logger

	mu     sync.Mutex
	owners map[string]*workspaceOwner
	tasks  sync.WaitGroup
	closed bool
}

func New(
	store Store,
	resolver Resolver,
	models Models,
	ids IDs,
	events Publisher,
	lifetime context.Context,
	logger *slog.Logger,
) (*Service, error) {
	if store == nil || resolver == nil || models == nil || ids == nil ||
		events == nil || lifetime == nil {
		return nil, errors.New("codebaseflow: dependencies are required")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		store:    store,
		resolver: resolver,
		models:   models,
		ids:      ids,
		events:   events,
		lifetime: lifetime,
		logger:   logger,
		owners:   make(map[string]*workspaceOwner),
	}, nil
}

func (service *Service) Status(
	ctx context.Context,
	request protocol.CodebaseStatusRequest,
) (*protocol.CodebaseStatus, error) {
	workspace, err := service.workspace(ctx, request.Workspace)
	if err != nil {
		return nil, err
	}
	index, err := service.store.GetCodebaseIndex(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if index.State != codebase.StateIndexing ||
		service.activeOperation(workspace) == index.OperationID {
		return present(index), nil
	}

	interrupted := index
	interrupted.State = codebase.StateError
	interrupted.OperationID = ""
	applied, err := service.store.FailCodebaseIndex(
		ctx,
		index.OperationID,
		interrupted,
	)
	if err != nil {
		return nil, err
	}
	if applied {
		service.publishChange()
	}
	index, err = service.store.GetCodebaseIndex(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return present(index), nil
}

func (service *Service) Reindex(
	ctx context.Context,
	request protocol.CodebaseReindexRequest,
) (*protocol.CodebaseReindexResponse, error) {
	workspace, err := service.workspace(ctx, request.Workspace)
	if err != nil {
		return nil, err
	}
	owner, err := service.ownerFor(workspace)
	if err != nil {
		return nil, err
	}

	owner.admission.Lock()
	defer owner.admission.Unlock()
	if service.isClosed() {
		return nil, errors.New("codebaseflow: closed")
	}
	operationID, err := service.ids.New("idx_")
	if err != nil {
		return nil, err
	}
	model, role, err := service.models.ResolveEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	previous, err := service.store.GetCodebaseIndex(ctx, workspace)
	if err != nil {
		return nil, err
	}
	index := previous
	index.Workspace = workspace
	index.State = codebase.StateIndexing
	index.OperationID = operationID
	index.ModelID = embeddingRoleID(role)
	if err := service.store.BeginCodebaseIndex(ctx, index); err != nil {
		return nil, err
	}

	if owner.cancel != nil {
		owner.cancel()
	}
	owner.generation++
	generation := owner.generation
	taskContext, cancel := context.WithCancel(service.lifetime)
	owner.operationID = operationID
	owner.cancel = cancel
	service.tasks.Add(1)
	service.publishChange()
	go service.build(taskContext, owner, generation, index, model)
	return &protocol.CodebaseReindexResponse{OperationID: operationID}, nil
}

func (service *Service) Search(
	ctx context.Context,
	request protocol.CodebaseSearchRequest,
) (*protocol.CodebaseSearchResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, fmt.Errorf("%w: query is required", protocol.ErrInvalidParams)
	}
	workspace, err := service.workspace(ctx, request.Workspace)
	if err != nil {
		return nil, err
	}
	index, err := service.store.GetCodebaseIndex(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if index.State != codebase.StateReady {
		return &protocol.CodebaseSearchResult{Hits: []protocol.CodebaseHit{}}, nil
	}
	model, role, err := service.models.ResolveEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	if current := embeddingRoleID(role); current != index.ModelID {
		return nil, fmt.Errorf(
			"%w: codebase index uses %q; reindex with %q before searching",
			protocol.ErrProviderError,
			index.ModelID,
			current,
		)
	}
	vectors, err := embedTexts(ctx, model, []string{query})
	if err != nil {
		return nil, err
	}
	documents, err := service.store.ListCodebaseDocuments(ctx, workspace)
	if err != nil {
		return nil, err
	}
	hits := rankDocuments(vectors[0], documents)
	limit := request.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	if len(hits) > limit {
		hits = slices.Clone(hits[:limit])
	}
	return &protocol.CodebaseSearchResult{Hits: hits}, nil
}

func (service *Service) Close() {
	if service == nil {
		return
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	service.closed = true
	owners := make([]*workspaceOwner, 0, len(service.owners))
	for _, owner := range service.owners {
		owners = append(owners, owner)
	}
	service.mu.Unlock()

	for _, owner := range owners {
		owner.admission.Lock()
		if owner.cancel != nil {
			owner.cancel()
		}
		owner.admission.Unlock()
	}
	service.tasks.Wait()
}

func (service *Service) build(
	ctx context.Context,
	owner *workspaceOwner,
	generation uint64,
	index codebase.Index,
	model embedding.Model,
) {
	defer service.tasks.Done()
	notifyAfterFinish := false
	defer func() {
		if service.finish(owner, generation) && notifyAfterFinish {
			service.publishChange()
		}
	}()

	corpus, buildErr := scanWorkspace(ctx, index.Workspace)
	if buildErr == nil {
		buildErr = embedDocuments(ctx, model, corpus.documents)
	}
	if ctx.Err() != nil {
		return
	}
	if buildErr != nil {
		service.logger.ErrorContext(
			ctx,
			"Codebase index build failed",
			"operationId", index.OperationID,
			"error", buildErr,
		)
	}

	owner.admission.Lock()
	if owner.generation == generation && owner.operationID == index.OperationID &&
		!service.isClosed() && ctx.Err() == nil {
		notifyAfterFinish = true
		persistContext, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			settlementTimeout,
		)
		var applied bool
		var settlementErr error
		if buildErr != nil {
			failed := index
			failed.State = codebase.StateError
			failed.OperationID = ""
			applied, settlementErr = service.store.FailCodebaseIndex(
				persistContext,
				index.OperationID,
				failed,
			)
		} else {
			now := time.Now().UTC()
			ready := index
			ready.State = codebase.StateReady
			ready.OperationID = ""
			ready.FileCount = corpus.fileCount
			ready.ChunkCount = len(corpus.documents)
			ready.Truncated = corpus.truncated
			ready.IndexedAt = &now
			applied, settlementErr = service.store.CompleteCodebaseIndex(
				persistContext,
				index.OperationID,
				ready,
				corpus.documents,
			)
		}
		if settlementErr != nil {
			service.logger.ErrorContext(
				persistContext,
				"Codebase index settlement failed",
				"operationId", index.OperationID,
				"error", settlementErr,
			)
		} else if !applied {
			service.logger.ErrorContext(
				persistContext,
				"Codebase index settlement lost its durable operation",
				"operationId", index.OperationID,
			)
		}
		cancel()
	}
	owner.admission.Unlock()
}

func (service *Service) ownerFor(workspace string) (*workspaceOwner, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return nil, errors.New("codebaseflow: closed")
	}
	owner := service.owners[workspace]
	if owner == nil {
		owner = &workspaceOwner{}
		service.owners[workspace] = owner
	}
	return owner, nil
}

func (service *Service) activeOperation(workspace string) string {
	service.mu.Lock()
	owner := service.owners[workspace]
	service.mu.Unlock()
	if owner == nil {
		return ""
	}
	owner.admission.Lock()
	defer owner.admission.Unlock()
	return owner.operationID
}

func (service *Service) finish(
	owner *workspaceOwner,
	generation uint64,
) bool {
	owner.admission.Lock()
	defer owner.admission.Unlock()
	if owner.generation != generation {
		return false
	}
	if owner.cancel != nil {
		owner.cancel()
	}
	owner.cancel = nil
	owner.operationID = ""
	return true
}

func (service *Service) isClosed() bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.closed
}

func (service *Service) workspace(
	ctx context.Context,
	ref protocol.WorkspaceRef,
) (string, error) {
	resolved, err := service.resolver.Resolve(ctx, ref.Path)
	if err != nil || !resolved.Available {
		return "", protocol.ErrWorkspaceUnavailable
	}
	return resolved.Workspace.Path(), nil
}

func (service *Service) publishChange() {
	service.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeCodebaseChanged})
}

func present(index codebase.Index) *protocol.CodebaseStatus {
	value := &protocol.CodebaseStatus{
		State:       protocol.CodebaseState(index.State),
		ModelID:     index.ModelID,
		FileCount:   index.FileCount,
		ChunkCount:  index.ChunkCount,
		Truncated:   index.Truncated,
		OperationID: index.OperationID,
	}
	if index.IndexedAt != nil {
		value.IndexedAt = index.IndexedAt.Format(time.RFC3339)
	}
	return value
}
