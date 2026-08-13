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

func (w *recordingAuthoredWatcher) Watch(
	scopes []AuthoredScope,
	resources []AuthoredResource,
	_ func(AuthoredResource),
) (AuthoredObservation, error) {
	w.scopes = scopes
	w.resources = resources
	return recordingAuthoredObservation{owner: w}, nil
}

type recordingAuthoredObservation struct{ owner *recordingAuthoredWatcher }

func (o recordingAuthoredObservation) Close() error { return nil }
func (o recordingAuthoredObservation) Accept(changes []AuthoredChange) error {
	o.owner.accepted = append(o.owner.accepted, changes...)
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
