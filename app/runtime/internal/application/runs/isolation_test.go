package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

type stubIsolation struct {
	path                   string
	err                    error
	gotSession, gotProject string
}

func (s *stubIsolation) Workspace(_ context.Context, sessionID, projectRoot string) (string, error) {
	s.gotSession, s.gotProject = sessionID, projectRoot
	return s.path, s.err
}

func TestExecutionCWD(t *testing.T) {
	ctx := context.Background()

	t.Run("non-isolated uses the project cwd", func(t *testing.T) {
		c := &Coordinator{isolation: &stubIsolation{path: "/should/not/be/used"}}
		cwd, isolated, err := c.executionCWD(ctx, sessionfixture.MustRestore(session.Snapshot{ID: "s1", Workspace: sessionfixture.MustWorkspace("/proj")}))
		if err != nil || cwd != "/proj" || isolated {
			t.Fatalf("executionCWD = (%q, %v, %v), want (/proj, false, nil)", cwd, isolated, err)
		}
	})

	t.Run("isolated resolves the sandbox copy", func(t *testing.T) {
		stub := &stubIsolation{path: "/tmp/copy"}
		c := &Coordinator{isolation: stub}
		cwd, isolated, err := c.executionCWD(ctx, sessionfixture.MustRestore(session.Snapshot{ID: "s1", Workspace: sessionfixture.MustWorkspace("/proj"), Isolated: true}))
		if err != nil || cwd != "/tmp/copy" || !isolated {
			t.Fatalf("executionCWD = (%q, %v, %v), want (/tmp/copy, true, nil)", cwd, isolated, err)
		}
		if stub.gotSession != "s1" || stub.gotProject != "/proj" {
			t.Fatalf("provider got (%q, %q), want (s1, /proj)", stub.gotSession, stub.gotProject)
		}
	})

	t.Run("isolated with no provider fails closed", func(t *testing.T) {
		c := &Coordinator{}
		value := sessionfixture.MustRestore(session.Snapshot{ID: "s1", Workspace: sessionfixture.MustWorkspace("/proj"), Isolated: true})
		if _, _, err := c.executionCWD(ctx, value); !errors.Is(err, ErrIsolationUnavailable) {
			t.Fatalf("err = %v, want ErrIsolationUnavailable", err)
		}
	})

	t.Run("isolated with a provider error fails closed", func(t *testing.T) {
		c := &Coordinator{isolation: &stubIsolation{err: errors.New("no backend")}}
		value := sessionfixture.MustRestore(session.Snapshot{ID: "s1", Workspace: sessionfixture.MustWorkspace("/proj"), Isolated: true})
		if _, _, err := c.executionCWD(ctx, value); !errors.Is(err, ErrIsolationUnavailable) {
			t.Fatalf("err = %v, want ErrIsolationUnavailable", err)
		}
	})
}
