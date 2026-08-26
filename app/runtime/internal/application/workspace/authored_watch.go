package workspace

import (
	"errors"
	"io"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
)

// ErrAuthoredWatchUnavailable reports that externally-authored workspace
// resource observation was not composed into this Runtime.
var ErrAuthoredWatchUnavailable = errors.New("workspace: authored resource watch unavailable")

// AuthoredResource is the closed set of file-backed product resources whose
// query projections can be changed by another process.
type AuthoredResource string

const (
	AuthoredKnowledge AuthoredResource = AuthoredResource(invalidation.Knowledge)
	AuthoredHooks     AuthoredResource = AuthoredResource(invalidation.Hooks)
	AuthoredSkills    AuthoredResource = AuthoredResource(invalidation.Skills)
)

// Valid reports whether a is one externally authored product source.
func (a AuthoredResource) Valid() bool {
	return a == AuthoredKnowledge || a == AuthoredHooks || a == AuthoredSkills
}

// InvalidationResource maps a to the same application-owned change
// vocabulary. Invalid resources map to the invalid zero value.
func (a AuthoredResource) InvalidationResource() invalidation.Resource {
	if !a.Valid() {
		return ""
	}
	return invalidation.Resource(a)
}

// AuthoredScope is one canonical workspace identity and its project root.
// Filesystem layout stays outside Application; these are the semantic roots
// already used by the Knowledge and Hooks use cases.
type AuthoredScope struct {
	Workspace   string
	ProjectRoot string
}

// AuthoredResourceWatcher adapts external filesystem state into semantic
// resource changes. Implementations own filenames, cascades, notification
// mechanisms, and symlink identity.
type AuthoredResourceWatcher interface {
	Watch(scopes []AuthoredScope, resources []AuthoredResource, notify func(AuthoredResource)) (AuthoredObservation, error)
}

// AuthoredChange identifies exact file-backed resource members that were
// changed through an authoritative use case.
type AuthoredChange struct {
	Resource   AuthoredResource
	Identities []string
}

// AuthoredObservation owns one live semantic resource observation. Accept
// records exact members after another authoritative path announced the change.
type AuthoredObservation interface {
	io.Closer
	Accept(changes []AuthoredChange) error
}

// AuthoredWatch resolves client workspace identities before delegating the
// external observation mechanism. It does not know transport topics.
type AuthoredWatch struct {
	scope      *Scope
	workspaces KnowledgeWorkspaceInspector
	watcher    AuthoredResourceWatcher
	mu         sync.Mutex
	active     map[*managedAuthoredObservation]struct{}
}

func NewAuthoredWatch(scope *Scope, workspaces KnowledgeWorkspaceInspector, watcher AuthoredResourceWatcher) *AuthoredWatch {
	return &AuthoredWatch{
		scope: scope, workspaces: workspaces, watcher: watcher,
		active: make(map[*managedAuthoredObservation]struct{}),
	}
}

// Watch starts one caller-owned observation. An empty cwd list still observes
// the implementation's global resource scopes.
func (a *AuthoredWatch) Watch(cwds []string, resources []AuthoredResource, notify func(AuthoredResource)) (AuthoredObservation, error) {
	if a == nil || a.watcher == nil {
		return nil, ErrAuthoredWatchUnavailable
	}
	if a.scope == nil || a.workspaces == nil {
		return nil, ErrCWDUnavailable
	}
	resources = distinctAuthoredResources(resources)
	if len(resources) == 0 {
		return nopAuthoredWatch{}, nil
	}
	seen := make(map[string]struct{}, len(cwds))
	scopes := make([]AuthoredScope, 0, len(cwds))
	for _, cwd := range cwds {
		root, err := a.scope.root(cwd)
		if err != nil {
			return nil, err
		}
		resolved, err := a.workspaces.Inspect(root)
		if err != nil {
			return nil, err
		}
		if resolved.Missing || resolved.ProjectRoot == "" {
			return nil, ErrCWDUnavailable
		}
		identity := root + "\x00" + resolved.ProjectRoot
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		scopes = append(scopes, AuthoredScope{Workspace: root, ProjectRoot: resolved.ProjectRoot})
	}
	inner, err := a.watcher.Watch(scopes, resources, notify)
	if err != nil {
		return nil, err
	}
	managed := &managedAuthoredObservation{owner: a, inner: inner}
	a.mu.Lock()
	a.active[managed] = struct{}{}
	a.mu.Unlock()
	return managed, nil
}

// Accept records exact members changed by a workspace use case before its
// invalidation is published. Failure is intentionally best-effort: the durable
// mutation already committed, and a later callback is a safe duplicate rather
// than grounds to report that commit as failed.
func (a *AuthoredWatch) Accept(change AuthoredChange) {
	if a == nil || len(change.Identities) == 0 {
		return
	}
	a.mu.Lock()
	active := make([]*managedAuthoredObservation, 0, len(a.active))
	for observation := range a.active {
		active = append(active, observation)
	}
	a.mu.Unlock()
	for _, observation := range active {
		_ = observation.inner.Accept([]AuthoredChange{change})
	}
}

type managedAuthoredObservation struct {
	owner *AuthoredWatch
	inner AuthoredObservation
	once  sync.Once
}

func (m *managedAuthoredObservation) Accept(changes []AuthoredChange) error {
	return m.inner.Accept(changes)
}

func (m *managedAuthoredObservation) Close() error {
	var err error
	m.once.Do(func() {
		m.owner.mu.Lock()
		delete(m.owner.active, m)
		m.owner.mu.Unlock()
		err = m.inner.Close()
	})
	return err
}

func distinctAuthoredResources(resources []AuthoredResource) []AuthoredResource {
	out := make([]AuthoredResource, 0, len(resources))
	for _, resource := range resources {
		if (resource == AuthoredKnowledge || resource == AuthoredHooks || resource == AuthoredSkills) && !slices.Contains(out, resource) {
			out = append(out, resource)
		}
	}
	return out
}

type nopAuthoredWatch struct{}

func (nopAuthoredWatch) Close() error                  { return nil }
func (nopAuthoredWatch) Accept([]AuthoredChange) error { return nil }
