// Package sessionflow coordinates Session use cases and their transaction-safe
// persistence. It owns authorization and mapping; the aggregate owns mutations.
package sessionflow

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type Store interface {
	CreateSession(context.Context, session.Session) error
	GetSession(context.Context, session.ID) (session.Session, error)
	GetSessionProjection(context.Context, session.ID) (session.Projection, error)
	ListSessionProjections(context.Context, int, *session.Cursor) (session.Page, error)
	UpdateSession(context.Context, session.Session, uint64) error
	DeleteSession(context.Context, session.ID) error
	LoadPlan(context.Context, string) (plandomain.State, error)
	ReadSessionMaterial(context.Context, session.ID) (Material, error)
	CreateSessionFork(context.Context, ForkWrite) error
	RollbackSessionHistory(context.Context, RollbackWrite) (session.Session, error)
	ReplaceSessionMaterial(context.Context, ImportWrite) error
}

type IDGenerator interface{ New(string) (string, error) }

type WorkspaceResolver interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type Checkpoints interface {
	Restore(context.Context, string, string, string) error
	DropSession(string) error
}

type Service struct {
	store      Store
	ids        IDGenerator
	workspaces WorkspaceResolver
	checkpoints Checkpoints
	now        func() time.Time
}

type Config struct {
	Store Store
	IDs IDGenerator
	Workspaces WorkspaceResolver
	Checkpoints Checkpoints
	Clock func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.IDs == nil || config.Workspaces == nil || config.Checkpoints == nil {
		return nil, errors.New("sessionflow: store, ids, workspaces and checkpoints are required")
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: config.Store, ids: config.IDs, workspaces: config.Workspaces, checkpoints: config.Checkpoints, now: clock}, nil
}

func (service *Service) Create(ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
	requested := ""
	if request.Workspace != nil {
		requested = request.Workspace.Path
	}
	resolved, err := service.workspaces.Resolve(ctx, requested)
	if err != nil || !resolved.Available {
		return nil, fmt.Errorf("%w: workspace %q is unavailable", protocol.ErrWorkspaceUnavailable, requested)
	}
	id, err := service.ids.New("ses_")
	if err != nil {
		return nil, err
	}
	value, err := session.New(session.Create{
		ID: session.ID(id), Title: request.Title, Workspace: resolved.Workspace, Now: service.now(),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err := service.store.CreateSession(ctx, value); err != nil {
		return nil, err
	}
	return present(value, resolved, session.StatusIdle), nil
}

func (service *Service) Get(ctx context.Context, id string) (*protocol.Session, error) {
	projection, err := service.store.GetSessionProjection(ctx, session.ID(id))
	if err != nil {
		return nil, projectLookup(err)
	}
	value := projection.Session
	resolved, err := service.workspaces.Resolve(ctx, value.Workspace().Path())
	if err != nil {
		return nil, err
	}
	return present(value, resolved, projection.Status), nil
}

func (service *Service) List(ctx context.Context, query protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	cursor, err := decodeCursor(query.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor is invalid", protocol.ErrInvalidParams)
	}
	page, err := service.store.ListSessionProjections(ctx, query.Limit, cursor)
	if err != nil {
		return nil, err
	}
	data := make([]protocol.Session, 0, len(page.Projections))
	for _, projection := range page.Projections {
		value := projection.Session
		resolved, err := service.workspaces.Resolve(ctx, value.Workspace().Path())
		if err != nil {
			return nil, err
		}
		data = append(data, *present(value, resolved, projection.Status))
	}
	next := ""
	if page.Next != nil {
		next = encodeCursor(*page.Next)
	}
	return protocol.NewPageWithCursor(data, next), nil
}

func (service *Service) Update(ctx context.Context, request protocol.UpdateSessionRequest) (*protocol.Session, error) {
	value, err := service.store.GetSession(ctx, session.ID(request.SessionID))
	if err != nil {
		return nil, projectLookup(err)
	}
	previous := value.Revision()
	patch := session.Patch{
		ExpectedRevision: request.ExpectedRevision, Title: request.Title,
		Model: request.Model, Favorite: request.Favorite, Now: service.now(),
	}
	var resolved workspacefs.Resolution
	if request.Workspace != nil {
		resolved, err = service.workspaces.Resolve(ctx, request.Workspace.Path)
		if err != nil || !resolved.Available {
			return nil, fmt.Errorf("%w: workspace %q is unavailable", protocol.ErrWorkspaceUnavailable, request.Workspace.Path)
		}
		patch.Workspace = &resolved.Workspace
	}
	if err := value.Update(patch); err != nil {
		if errors.Is(err, session.ErrRevisionConflict) {
			return nil, fmt.Errorf("%w: %v", protocol.ErrRevisionConflict, err)
		}
		return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err := service.store.UpdateSession(ctx, value, previous); err != nil {
		if errors.Is(err, session.ErrRevisionConflict) {
			return nil, fmt.Errorf("%w: session changed concurrently", protocol.ErrRevisionConflict)
		}
		return nil, projectLookup(err)
	}
	projection, err := service.store.GetSessionProjection(ctx, value.ID())
	if err != nil {
		return nil, projectLookup(err)
	}
	resolved, err = service.workspaces.Resolve(ctx, projection.Session.Workspace().Path())
	if err != nil {
		return nil, err
	}
	return present(projection.Session, resolved, projection.Status), nil
}

func (service *Service) Delete(ctx context.Context, id string) error {
	if err := service.store.DeleteSession(ctx, session.ID(id)); err != nil {
		return projectLookup(err)
	}
	return service.checkpoints.DropSession(id)
}

func (service *Service) ResolveWorkspace(ctx context.Context, requested string) (*protocol.WorkspaceInfo, error) {
	resolved, err := service.workspaces.Resolve(ctx, requested)
	if err != nil {
		return nil, err
	}
	availability := protocol.WorkspaceMissing
	if resolved.Available {
		availability = protocol.WorkspaceAvailable
	}
	return &protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: resolved.Workspace.Path()},
		ProjectRoot: resolved.ProjectRoot, Availability: availability,
	}, nil
}

func (service *Service) ListWorkspaces(ctx context.Context) (*protocol.Page[protocol.WorkspaceSummary], error) {
	type aggregate struct {
		info  protocol.WorkspaceInfo
		count int
		last  time.Time
	}
	byPath := make(map[string]*aggregate)
	var cursor *session.Cursor
	for {
		page, err := service.store.ListSessionProjections(ctx, 200, cursor)
		if err != nil {
			return nil, err
		}
		for _, projection := range page.Projections {
			value := projection.Session
			path := value.Workspace().Path()
			entry := byPath[path]
			if entry == nil {
				resolved, err := service.workspaces.Resolve(ctx, path)
				if err != nil {
					return nil, err
				}
				availability := protocol.WorkspaceMissing
				if resolved.Available {
					availability = protocol.WorkspaceAvailable
				}
				entry = &aggregate{info: protocol.WorkspaceInfo{
					Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: resolved.ProjectRoot,
					Availability: availability,
				}}
				byPath[path] = entry
			}
			entry.count++
			if value.UpdatedAt().After(entry.last) {
				entry.last = value.UpdatedAt()
			}
		}
		if page.Next == nil {
			break
		}
		cursor = page.Next
	}
	data := make([]protocol.WorkspaceSummary, 0, len(byPath))
	for _, entry := range byPath {
		last := entry.last
		data = append(data, protocol.WorkspaceSummary{
			Workspace: entry.info, Name: filepath.Base(entry.info.Ref.Path),
			SessionCount: entry.count, LastActiveAt: &last,
		})
	}
	slices.SortFunc(data, func(left, right protocol.WorkspaceSummary) int {
		if order := right.LastActiveAt.Compare(*left.LastActiveAt); order != 0 {
			return order
		}
		return strings.Compare(left.Workspace.Ref.Path, right.Workspace.Ref.Path)
	})
	return protocol.NewPage(data), nil
}

func present(value session.Session, resolved workspacefs.Resolution, status session.Status) *protocol.Session {
	availability := protocol.WorkspaceMissing
	if resolved.Available {
		availability = protocol.WorkspaceAvailable
	}
	return &protocol.Session{
		ID: value.ID().String(), Title: value.Title(), Status: protocol.SessionStatus(status), Model: value.Model(),
		Workspace: protocol.WorkspaceInfo{
			Ref: protocol.WorkspaceRef{Path: resolved.Workspace.Path()},
			ProjectRoot: resolved.ProjectRoot, Availability: availability,
		},
		CreatedAt: value.CreatedAt(), UpdatedAt: value.UpdatedAt(),
		Favorite: value.Favorite(), Revision: value.Revision(),
	}
}

func projectLookup(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	return err
}

func encodeCursor(cursor session.Cursor) string {
	favorite := "0"
	if cursor.Favorite {
		favorite = "1"
	}
	value := favorite + "\n" + cursor.UpdatedAt.UTC().Format(time.RFC3339Nano) + "\n" + cursor.ID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursor(value string) (*session.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "\n", 3)
	if len(parts) != 3 || parts[2] == "" || (parts[0] != "0" && parts[0] != "1") {
		return nil, errors.New("invalid cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return nil, err
	}
	return &session.Cursor{Favorite: parts[0] == "1", UpdatedAt: updatedAt, ID: parts[2]}, nil
}
