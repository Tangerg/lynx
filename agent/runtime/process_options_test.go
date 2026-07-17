package runtime

import (
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

func TestSnapshotProcessOptionsOwnsMutableContainers(t *testing.T) {
	firstExtension := &constructorExtension{name: "first"}
	secondExtension := &constructorExtension{name: "second"}
	extensions := []core.Extension{firstExtension}
	session := core.NewSession("session-1", "user-1", "agent-1")
	session.Metadata["host"] = map[string]any{"mutable": true}
	callMiddleware := func(next chat.Model) chat.Model { return next }
	streamMiddleware := func(next chat.Streamer) chat.Streamer { return next }
	guardrails := &core.ChatGuardrails{
		CallMiddlewares:   []chat.CallMiddleware{callMiddleware},
		StreamMiddlewares: []chat.StreamMiddleware{streamMiddleware},
		MaxToolRounds:     4,
	}

	snapshot, err := snapshotProcessOptions(core.ProcessOptions{
		Session:    &session,
		Extensions: extensions,
		Guardrails: guardrails,
	})
	if err != nil {
		t.Fatalf("snapshotProcessOptions: %v", err)
	}

	extensions[0] = secondExtension
	session.ID = "mutated"
	session.Metadata["later"] = true
	guardrails.CallMiddlewares[0] = nil
	guardrails.StreamMiddlewares[0] = nil
	guardrails.MaxToolRounds = 99

	if len(snapshot.extensions) != 1 || snapshot.extensions[0] != firstExtension {
		t.Fatalf("extensions = %#v, want original extension", snapshot.extensions)
	}
	if snapshot.session == nil || snapshot.session.ID != "session-1" {
		t.Fatalf("session = %#v, want session-1 snapshot", snapshot.session)
	}
	if snapshot.session.Metadata != nil {
		t.Fatalf("process retained host-owned session metadata: %#v", snapshot.session.Metadata)
	}
	if snapshot.guardrails == nil || snapshot.guardrails.MaxToolRounds != 4 {
		t.Fatalf("guardrails = %#v, want MaxToolRounds 4", snapshot.guardrails)
	}
	if snapshot.guardrails.CallMiddlewares[0] == nil || snapshot.guardrails.StreamMiddlewares[0] == nil {
		t.Fatal("guardrail middleware slices alias caller storage")
	}
	if snapshot.budget != core.DefaultBudget() {
		t.Fatalf("budget = %#v, want default %#v", snapshot.budget, core.DefaultBudget())
	}
}

func TestSnapshotProcessOptionsSeparatesConcurrentCallerMutation(t *testing.T) {
	firstExtension := &constructorExtension{name: "first"}
	secondExtension := &constructorExtension{name: "second"}
	extensions := []core.Extension{firstExtension}
	session := core.NewSession("session-1", "user-1", "agent-1")
	guardrails := &core.ChatGuardrails{
		CallMiddlewares: []chat.CallMiddleware{func(next chat.Model) chat.Model { return next }},
		MaxToolRounds:   2,
	}
	snapshot, err := snapshotProcessOptions(core.ProcessOptions{
		Session:    &session,
		Extensions: extensions,
		Guardrails: guardrails,
	})
	if err != nil {
		t.Fatalf("snapshotProcessOptions: %v", err)
	}

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for index := range 1_000 {
			extensions[0] = secondExtension
			session.ID = "mutated"
			guardrails.MaxToolRounds = index
			guardrails.CallMiddlewares[0] = nil
		}
	}()
	go func() {
		defer group.Done()
		for range 1_000 {
			_ = snapshot.extensions[0]
			_ = snapshot.session.ID
			_ = snapshot.guardrails.MaxToolRounds
			_ = snapshot.guardrails.CallMiddlewares[0]
		}
	}()
	group.Wait()

	if snapshot.extensions[0] != firstExtension || snapshot.session.ID != "session-1" ||
		snapshot.guardrails.MaxToolRounds != 2 || snapshot.guardrails.CallMiddlewares[0] == nil {
		t.Fatalf("snapshot changed with caller state: %#v", snapshot)
	}
}

func TestSnapshotProcessOptionsRejectsInvalidCapabilities(t *testing.T) {
	var nilBlackboard *inMemoryBlackboard
	var nilExtension *constructorExtension
	for _, test := range []struct {
		name     string
		options  core.ProcessOptions
		contains string
	}{
		{
			name:     "typed nil blackboard",
			options:  core.ProcessOptions{Blackboard: nilBlackboard},
			contains: "Blackboard is typed nil",
		},
		{
			name:     "typed nil extension",
			options:  core.ProcessOptions{Extensions: []core.Extension{nilExtension}},
			contains: "Extensions[0] is nil",
		},
		{
			name: "negative tool rounds",
			options: core.ProcessOptions{Guardrails: &core.ChatGuardrails{
				MaxToolRounds: -1,
			}},
			contains: "MaxToolRounds must not be negative",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := snapshotProcessOptions(test.options)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestNewSnapshotsEngineGuardrails(t *testing.T) {
	callMiddleware := func(next chat.Model) chat.Model { return next }
	guardrails := &core.ChatGuardrails{
		CallMiddlewares: []chat.CallMiddleware{callMiddleware},
		MaxToolRounds:   3,
	}
	engine, err := New(Config{Guardrails: guardrails})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	guardrails.CallMiddlewares[0] = nil
	guardrails.MaxToolRounds = 9
	if engine.guardrails == nil || engine.guardrails.MaxToolRounds != 3 {
		t.Fatalf("engine guardrails = %#v, want independent snapshot", engine.guardrails)
	}
	if engine.guardrails.CallMiddlewares[0] == nil {
		t.Fatal("engine guardrails retained caller middleware slice")
	}
}

func TestNewRejectsNegativeEngineGuardrailRounds(t *testing.T) {
	engine, err := New(Config{Guardrails: &core.ChatGuardrails{MaxToolRounds: -1}})
	if engine != nil || err == nil || !strings.Contains(err.Error(), "MaxToolRounds must not be negative") {
		t.Fatalf("New = %#v, %v; want nil engine and guardrail error", engine, err)
	}
}

func TestChildExtensionsDoesNotReuseCallerSlice(t *testing.T) {
	first := &constructorExtension{name: "first"}
	second := &constructorExtension{name: "second"}
	caller := make([]core.Extension, 1, 2)
	caller[0] = first

	child := (&Process{}).childExtensions(caller)
	child[0] = second
	if caller[0] != first {
		t.Fatal("child extension merge mutated caller-owned slice")
	}
}

func TestChildExtensionsLeavesTypedNilForConstructionValidation(t *testing.T) {
	var nilExtension *constructorExtension
	merged := (&Process{}).childExtensions([]core.Extension{nilExtension})

	_, err := snapshotProcessOptions(core.ProcessOptions{Extensions: merged})
	if err == nil || !strings.Contains(err.Error(), "Extensions[0] is nil") {
		t.Fatalf("snapshotProcessOptions error = %v, want typed-nil extension error", err)
	}
}
