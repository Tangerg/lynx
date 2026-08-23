// Package memoryflow owns AgentMemory management, effective recall, and
// retrieval. Persistence and model adapters remain behind consumer-shaped
// ports; the Lyra wire is projected only at this application boundary.
package memoryflow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/embedding"

	"github.com/Tangerg/lynx/app2/runtime/domain/agentmemory"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const (
	defaultSearchLimit    = 8
	maxSearchLimit        = 20
	maxSemanticCandidates = 128
)

type Store interface {
	ListAgentMemory(context.Context, agentmemory.Scope, string) ([]agentmemory.Item, error)
	AddAgentMemory(context.Context, agentmemory.Item) (agentmemory.Item, bool, error)
	ReviewAgentMemory(
		context.Context,
		string,
		agentmemory.ReviewDecision,
		time.Time,
	) (bool, error)
	UpdateAgentMemory(
		context.Context,
		string,
		agentmemory.Patch,
		time.Time,
	) (agentmemory.Item, bool, error)
	DeleteAgentMemory(context.Context, string) (bool, error)
}

type Resolver interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type IDs interface {
	New(string) (string, error)
}

type Publisher interface {
	Publish(protocol.RuntimeEvent)
}

// EmbeddingModels is optional. Search remains lexically complete when the
// embedding role is unset or unhealthy.
type EmbeddingModels interface {
	ResolveMemoryEmbedding(context.Context) (embedding.Model, error)
}

type Config struct {
	Store    Store
	Resolver Resolver
	IDs      IDs
	Events   Publisher
	Models   EmbeddingModels
	Clock    func() time.Time
}

type Service struct {
	store    Store
	resolver Resolver
	ids      IDs
	events   Publisher
	models   EmbeddingModels
	now      func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.Resolver == nil || config.IDs == nil || config.Events == nil {
		return nil, errors.New("memoryflow: store, resolver, ids and events are required")
	}
	now := config.Clock
	if now == nil {
		now = time.Now
	}
	return &Service{
		store: config.Store, resolver: config.Resolver, ids: config.IDs,
		events: config.Events, models: config.Models, now: now,
	}, nil
}

func (service *Service) List(
	ctx context.Context,
	request protocol.AgentMemoryListRequest,
) (*protocol.AgentMemoryList, error) {
	scope, project, err := service.target(ctx, request.Scope, request.Workspace)
	if err != nil {
		return nil, err
	}
	items, err := service.store.ListAgentMemory(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	values := make([]protocol.AgentMemoryItem, len(items))
	for index, item := range items {
		values[index] = present(item)
	}
	return &protocol.AgentMemoryList{Items: values}, nil
}

func (service *Service) Add(
	ctx context.Context,
	request protocol.AgentMemoryAddRequest,
) (*protocol.AgentMemoryItem, error) {
	scope, project, err := service.target(ctx, request.Scope, request.Workspace)
	if err != nil {
		return nil, err
	}
	id, err := service.ids.New("mem_")
	if err != nil {
		return nil, err
	}
	item, err := agentmemory.NewUserItem(
		id, scope, project, request.Content, service.now(),
	)
	if err != nil {
		return nil, invalid(err)
	}
	saved, changed, err := service.store.AddAgentMemory(ctx, item)
	if err != nil {
		return nil, projectError(err)
	}
	if changed {
		service.publish()
	}
	value := present(saved)
	return &value, nil
}

func (service *Service) Review(
	ctx context.Context,
	request protocol.AgentMemoryReviewRequest,
) error {
	decision := agentmemory.ReviewDecision(request.Decision)
	if decision != agentmemory.ReviewApprove && decision != agentmemory.ReviewReject {
		return invalid(errors.New("decision must be approve or reject"))
	}
	changed, err := service.store.ReviewAgentMemory(
		ctx, request.ID, decision, service.now(),
	)
	if err != nil {
		return projectError(err)
	}
	if changed {
		service.publish()
	}
	return nil
}

func (service *Service) Update(
	ctx context.Context,
	request protocol.AgentMemoryUpdateRequest,
) (*protocol.AgentMemoryItem, error) {
	if request.Content == nil && request.Pinned == nil {
		return nil, invalid(errors.New("update has no changes"))
	}
	saved, changed, err := service.store.UpdateAgentMemory(
		ctx,
		request.ID,
		agentmemory.Patch{Content: request.Content, Pinned: request.Pinned},
		service.now(),
	)
	if err != nil {
		return nil, projectError(err)
	}
	if changed {
		service.publish()
	}
	value := present(saved)
	return &value, nil
}

func (service *Service) Delete(ctx context.Context, id string) error {
	changed, err := service.store.DeleteAgentMemory(ctx, id)
	if err != nil {
		return projectError(err)
	}
	if changed {
		service.publish()
	}
	return nil
}

// Effective returns the active user + project memory visible to a fresh Run.
// Pinned values lead, then the most recently updated values.
func (service *Service) Effective(
	ctx context.Context,
	workspace string,
) ([]agentmemory.Item, error) {
	project, err := service.project(ctx, workspace)
	if err != nil {
		return nil, err
	}
	user, err := service.store.ListAgentMemory(ctx, agentmemory.ScopeUser, "")
	if err != nil {
		return nil, err
	}
	projectItems, err := service.store.ListAgentMemory(
		ctx, agentmemory.ScopeProject, project,
	)
	if err != nil {
		return nil, err
	}
	values := make([]agentmemory.Item, 0, len(user)+len(projectItems))
	for _, group := range [][]agentmemory.Item{user, projectItems} {
		for _, item := range group {
			if item.Status == agentmemory.StatusActive {
				values = append(values, item)
			}
		}
	}
	slices.SortStableFunc(values, compareEffective)
	return values, nil
}

func (service *Service) Search(
	ctx context.Context,
	workspace string,
	query string,
	limit int,
) ([]agentmemory.Item, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, invalid(errors.New("query is required"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	items, err := service.Effective(ctx, workspace)
	if err != nil {
		return nil, err
	}
	candidates := make([]agentmemory.Candidate, len(items))
	for index, item := range items {
		candidates[index].Item = item
	}
	queryVector := service.semanticVectors(ctx, query, candidates)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return agentmemory.Rank(query, queryVector, candidates, limit), nil
}

func (service *Service) semanticVectors(
	ctx context.Context,
	query string,
	candidates []agentmemory.Candidate,
) []float64 {
	if service.models == nil || len(candidates) == 0 {
		return nil
	}
	model, err := service.models.ResolveMemoryEmbedding(ctx)
	if err != nil || model == nil {
		return nil
	}
	count := min(len(candidates), maxSemanticCandidates)
	texts := make([]string, count+1)
	texts[0] = query
	for index := 0; index < count; index++ {
		texts[index+1] = candidates[index].Item.Content
	}
	request, err := embedding.NewRequest(texts)
	if err != nil {
		return nil
	}
	response, err := model.Call(ctx, request)
	if err != nil || response == nil || response.Validate() != nil || len(response.Results) != len(texts) {
		return nil
	}
	dimensions := len(response.Results[0].Embedding)
	if dimensions == 0 {
		return nil
	}
	for index := 0; index < count; index++ {
		vector := response.Results[index+1].Embedding
		if len(vector) != dimensions {
			return nil
		}
		candidates[index].Vector = slices.Clone(vector)
	}
	return slices.Clone(response.Results[0].Embedding)
}

func (service *Service) target(
	ctx context.Context,
	scope protocol.AgentMemoryScope,
	workspace *protocol.WorkspaceRef,
) (agentmemory.Scope, string, error) {
	switch scope {
	case protocol.AgentMemoryScopeUser:
		if workspace != nil {
			return "", "", invalid(errors.New("user scope forbids workspace"))
		}
		return agentmemory.ScopeUser, "", nil
	case protocol.AgentMemoryScopeProject:
		if workspace == nil {
			return "", "", invalid(errors.New("project scope requires workspace"))
		}
		project, err := service.project(ctx, workspace.Path)
		return agentmemory.ScopeProject, project, err
	default:
		return "", "", invalid(fmt.Errorf("unknown scope %q", scope))
	}
}

func (service *Service) project(ctx context.Context, workspace string) (string, error) {
	resolved, err := service.resolver.Resolve(ctx, workspace)
	if err != nil {
		if cancellation := ctx.Err(); cancellation != nil {
			return "", cancellation
		}
		return "", protocol.ErrWorkspaceUnavailable
	}
	if !resolved.Available {
		return "", protocol.ErrWorkspaceUnavailable
	}
	if resolved.ProjectRoot != "" {
		return resolved.ProjectRoot, nil
	}
	return resolved.Workspace.Path(), nil
}

func (service *Service) publish() {
	service.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeAgentMemoryChanged})
}

func compareEffective(left agentmemory.Item, right agentmemory.Item) int {
	if left.Pinned != right.Pinned {
		if left.Pinned {
			return -1
		}
		return 1
	}
	if order := right.UpdatedAt.Compare(left.UpdatedAt); order != 0 {
		return order
	}
	return strings.Compare(left.ID, right.ID)
}

func present(item agentmemory.Item) protocol.AgentMemoryItem {
	return protocol.AgentMemoryItem{
		ID: item.ID, Scope: protocol.AgentMemoryScope(item.Scope),
		Content: item.Content, Origin: protocol.AgentMemoryOrigin(item.Origin),
		Status: protocol.AgentMemoryStatus(item.Status), Pinned: item.Pinned,
		SessionID: item.SessionID, Day: item.Day,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func invalid(err error) error {
	return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
}

func projectError(err error) error {
	switch {
	case errors.Is(err, agentmemory.ErrNotFound):
		return protocol.ErrItemNotFound
	case errors.Is(err, agentmemory.ErrNotPending),
		errors.Is(err, agentmemory.ErrDuplicate),
		errors.Is(err, agentmemory.ErrTargetFull),
		errors.Is(err, agentmemory.ErrInvalidMutation):
		return invalid(err)
	default:
		return err
	}
}
