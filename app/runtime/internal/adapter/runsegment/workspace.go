package runsegment

import (
	"slices"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
)

// FileChangePublisher nudges live workspace subscribers after a tool-owned file
// mutation. It is deliberately path-only so persistence does not acquire event
// presentation responsibilities.
type FileChangePublisher func(workspaceapp.FileChangeNotice)

// WorkspaceNotifier adapts the live publisher to the Run application's narrow
// notification port. It owns no durable state and never joins a transaction.
type WorkspaceNotifier struct {
	publish FileChangePublisher
}

var _ runs.WorkspaceChangeNotifier = WorkspaceNotifier{}

func NewWorkspaceNotifier(publish FileChangePublisher) WorkspaceNotifier {
	return WorkspaceNotifier{publish: publish}
}

func (w WorkspaceNotifier) Nudge(cwd string, paths []string) {
	if w.publish != nil && len(paths) > 0 {
		w.publish(workspaceapp.FileChangeNotice{CWD: cwd, Paths: slices.Clone(paths)})
	}
}
