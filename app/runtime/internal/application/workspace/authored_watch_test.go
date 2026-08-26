package workspace

import (
	"reflect"
	"testing"
)

type recordingAuthoredWatcher struct {
	scopes    []AuthoredScope
	resources []AuthoredResource
	accepted  []AuthoredChange
}

func (r *recordingAuthoredWatcher) Watch(
	scopes []AuthoredScope,
	resources []AuthoredResource,
	_ func(AuthoredResource),
) (AuthoredObservation, error) {
	r.scopes = scopes
	r.resources = resources
	return recordingAuthoredObservation{owner: r}, nil
}

type recordingAuthoredObservation struct{ owner *recordingAuthoredWatcher }

func (r recordingAuthoredObservation) Close() error { return nil }
func (r recordingAuthoredObservation) Accept(changes []AuthoredChange) error {
	r.owner.accepted = append(r.owner.accepted, changes...)
	return nil
}

func TestAuthoredWatchResolvesAndDeduplicatesScopes(t *testing.T) {
	root := t.TempDir()
	watcher := &recordingAuthoredWatcher{}
	useCases := NewAuthoredWatch(NewScope(root, root, testPaths{}), staticWorkspaceInspector{
		resolved: Resolved{Path: root, ProjectRoot: root},
	}, watcher)
	closer, err := useCases.Watch(
		[]string{"", root},
		[]AuthoredResource{AuthoredKnowledge, AuthoredKnowledge, AuthoredHooks, AuthoredSkills},
		func(AuthoredResource) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()
	if !reflect.DeepEqual(watcher.scopes, []AuthoredScope{{Workspace: root, ProjectRoot: root}}) {
		t.Fatalf("scopes = %+v", watcher.scopes)
	}
	if !reflect.DeepEqual(watcher.resources, []AuthoredResource{AuthoredKnowledge, AuthoredHooks, AuthoredSkills}) {
		t.Fatalf("resources = %+v", watcher.resources)
	}
}

type staticWorkspaceInspector struct{ resolved Resolved }

func (s staticWorkspaceInspector) Inspect(string) (Resolved, error) { return s.resolved, nil }
