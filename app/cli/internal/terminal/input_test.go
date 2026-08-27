package terminal

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/agent/mock"
	"github.com/Tangerg/scope/app/cli/internal/workbench"
)

func TestHandledNoOpEditsCannotStarveDraftAutosave(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: t.TempDir(), StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	sessionID := firstRuntimeSession(t, backend)
	host.Type("draft survives no-op edits")
	for range len("draft survives no-op edits") {
		host.Send(input.Key{Code: input.Left})
	}

	started := time.Now()
	deadline := started.Add(2 * time.Second)
	var draft agent.Message
	var found bool
	for time.Now().Before(deadline) {
		host.Send(input.Key{Code: input.Backspace})
		if time.Since(started) >= 5*draftPersistenceDelay {
			var err error
			draft, found, err = storedDraft(stateDirectory, sessionID)
			if err != nil {
				t.Fatal(err)
			}
			if found && draft.Equal(agent.Message{Text: "draft survives no-op edits"}) {
				break
			}
		}
		time.Sleep(draftPersistenceDelay / 10)
	}
	if !found || !draft.Equal(agent.Message{Text: "draft survives no-op edits"}) {
		t.Fatalf("autosaved draft = (%+v, %v), want the authored value while no-op edits continue", draft, found)
	}
	stop()
}

func TestResolvedKeyTextSchedulesDraftAutosave(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: t.TempDir(), StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	sessionID := firstRuntimeSession(t, backend)

	host.Send(input.Key{Text: "你好"})
	host.Shows(t, "你好")
	awaitStoredDraft(t, stateDirectory, sessionID, agent.Message{Text: "你好"})
	stop()
}

func TestProgrammaticComposerEditsScheduleDraftAutosave(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testing.T, *programtest.Host)
		want string
	}{
		{
			name: "command palette",
			edit: func(t *testing.T, host *programtest.Host) {
				host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
				host.Shows(t, "Commands")
				host.Type("rename")
				host.Press(input.Enter)
			},
			want: "/rename",
		},
		{
			name: "completion",
			edit: func(_ *testing.T, host *programtest.Host) {
				host.Type("/sho")
				host.Press(input.Enter)
			},
			want: "/shortcuts",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := mock.New()
			backend.Instant = true
			stateDirectory := t.TempDir()
			host, stop := runUIFromConfig(t, Config{
				Runtime: backend, Workspace: t.TempDir(), StateDirectory: stateDirectory,
			})
			host.Shows(t, "Ask lyra")
			sessionID := firstRuntimeSession(t, backend)

			test.edit(t, host)
			host.Shows(t, test.want)
			awaitStoredDraft(t, stateDirectory, sessionID, agent.Message{Text: test.want})
			stop()
		})
	}
}

func awaitStoredDraft(t *testing.T, stateDirectory, sessionID string, want agent.Message) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var draft agent.Message
	var found bool
	var err error
	for time.Now().Before(deadline) {
		draft, found, err = storedDraft(stateDirectory, sessionID)
		if err == nil && found && draft.Equal(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("autosaved draft = (%+v, %v, %v), want %+v", draft, found, err, want)
}

func storedDraft(stateDirectory, sessionID string) (agent.Message, bool, error) {
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		return agent.Message{}, false, err
	}
	return store.Draft(sessionID)
}
