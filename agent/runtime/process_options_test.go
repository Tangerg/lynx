package runtime

import (
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

type engineOnlyValidator struct{}

func (engineOnlyValidator) Name() string               { return "validator" }
func (engineOnlyValidator) Validate(*core.Agent) error { return nil }

func TestSnapshotProcessOptionsOwnsMutableContainers(t *testing.T) {
	firstExtension := &constructorExtension{name: "first"}
	secondExtension := &constructorExtension{name: "second"}
	extensions := []core.Extension{firstExtension}
	callMiddleware := func(next chat.Model) chat.Model { return next }
	streamMiddleware := func(next chat.Streamer) chat.Streamer { return next }
	middleware := &core.ChatMiddleware{
		CallMiddlewares:   []chat.CallMiddleware{callMiddleware},
		StreamMiddlewares: []chat.StreamMiddleware{streamMiddleware},
	}

	snapshot, err := snapshotProcessOptions(core.ProcessOptions{
		Extensions:     extensions,
		ChatMiddleware: middleware,
		MaxToolRounds:  4,
	})
	if err != nil {
		t.Fatalf("snapshotProcessOptions: %v", err)
	}

	extensions[0] = secondExtension
	middleware.CallMiddlewares[0] = nil
	middleware.StreamMiddlewares[0] = nil

	if len(snapshot.extensions) != 1 || snapshot.extensions[0].value != firstExtension {
		t.Fatalf("extensions = %#v, want original extension", snapshot.extensions)
	}
	if snapshot.chatMiddleware == nil || snapshot.maxToolRounds != 4 {
		t.Fatalf("chat config = %#v/%d, want middleware and MaxToolRounds 4", snapshot.chatMiddleware, snapshot.maxToolRounds)
	}
	if snapshot.chatMiddleware.CallMiddlewares[0] == nil || snapshot.chatMiddleware.StreamMiddlewares[0] == nil {
		t.Fatal("chat middleware slices alias caller storage")
	}
	if snapshot.budget != (core.Budget{}) {
		t.Fatalf("budget = %#v, want unbounded zero value", snapshot.budget)
	}
}

func TestSnapshotProcessOptionsSeparatesConcurrentCallerMutation(t *testing.T) {
	firstExtension := &constructorExtension{name: "first"}
	secondExtension := &constructorExtension{name: "second"}
	extensions := []core.Extension{firstExtension}
	middleware := &core.ChatMiddleware{
		CallMiddlewares: []chat.CallMiddleware{func(next chat.Model) chat.Model { return next }},
	}
	options := core.ProcessOptions{Extensions: extensions, ChatMiddleware: middleware, MaxToolRounds: 2}
	snapshot, err := snapshotProcessOptions(options)
	if err != nil {
		t.Fatalf("snapshotProcessOptions: %v", err)
	}

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for index := range 1_000 {
			extensions[0] = secondExtension
			options.MaxToolRounds = index
			middleware.CallMiddlewares[0] = nil
		}
	}()
	go func() {
		defer group.Done()
		for range 1_000 {
			_ = snapshot.extensions[0].value
			_ = snapshot.maxToolRounds
			_ = snapshot.chatMiddleware.CallMiddlewares[0]
		}
	}()
	group.Wait()

	if snapshot.extensions[0].value != firstExtension ||
		snapshot.maxToolRounds != 2 || snapshot.chatMiddleware.CallMiddlewares[0] == nil {
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
			name:     "extension without process capability",
			options:  core.ProcessOptions{Extensions: []core.Extension{nameOnlyExtension{name: "empty"}}},
			contains: "no process-scoped capability",
		},
		{
			name:     "engine ID generator",
			options:  core.ProcessOptions{Extensions: []core.Extension{core.NewUUIDGenerator("ids")}},
			contains: "engine-only capabilities: IDGenerator",
		},
		{
			name:     "engine blackboard prototype",
			options:  core.ProcessOptions{Extensions: []core.Extension{newInMemoryBlackboard()}},
			contains: "engine-only capabilities: Blackboard",
		},
		{
			name:     "engine agent validator",
			options:  core.ProcessOptions{Extensions: []core.Extension{engineOnlyValidator{}}},
			contains: "engine-only capabilities: AgentValidator",
		},
		{
			name:     "negative tool rounds",
			options:  core.ProcessOptions{MaxToolRounds: -1},
			contains: "MaxToolRounds must not be negative",
		},
		{
			name:     "negative action budget",
			options:  core.ProcessOptions{Budget: core.Budget{ActionLimit: -1}},
			contains: "action limit must not be negative",
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

func TestNewSnapshotsEngineChatMiddleware(t *testing.T) {
	callMiddleware := func(next chat.Model) chat.Model { return next }
	middleware := &core.ChatMiddleware{
		CallMiddlewares: []chat.CallMiddleware{callMiddleware},
	}
	engine, err := New(Config{ChatMiddleware: middleware, MaxToolRounds: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	middleware.CallMiddlewares[0] = nil
	if engine.chatMiddleware == nil || engine.maxToolRounds != 3 {
		t.Fatalf("engine chat config = %#v/%d, want independent snapshot", engine.chatMiddleware, engine.maxToolRounds)
	}
	if engine.chatMiddleware.CallMiddlewares[0] == nil {
		t.Fatal("engine chat middleware retained caller slice")
	}
}

func TestNewRejectsNegativeEngineToolRounds(t *testing.T) {
	engine, err := New(Config{MaxToolRounds: -1})
	if engine != nil || err == nil || !strings.Contains(err.Error(), "MaxToolRounds must not be negative") {
		t.Fatalf("New = %#v, %v; want nil engine and tool-round error", engine, err)
	}
}

func TestChildExtensionsDoesNotReuseCallerSlice(t *testing.T) {
	first := &constructorExtension{name: "first"}
	second := &constructorExtension{name: "second"}
	caller := make([]core.Extension, 1, 2)
	caller[0] = first

	child, err := (&Process{}).childExtensions(caller)
	if err != nil {
		t.Fatalf("childExtensions: %v", err)
	}
	child[0] = second
	if caller[0] != first {
		t.Fatal("child extension merge mutated caller-owned slice")
	}
}

func TestChildExtensionsLeavesTypedNilForConstructionValidation(t *testing.T) {
	var nilExtension *constructorExtension
	merged, err := (&Process{}).childExtensions([]core.Extension{nilExtension})
	if err != nil {
		t.Fatalf("childExtensions: %v", err)
	}

	_, err = snapshotProcessOptions(core.ProcessOptions{Extensions: merged})
	if err == nil || !strings.Contains(err.Error(), "Extensions[0] is nil") {
		t.Fatalf("snapshotProcessOptions error = %v, want typed-nil extension error", err)
	}
}
