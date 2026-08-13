package terminal

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
)

type agentMemoryServiceStub struct {
	mu      sync.Mutex
	project []agentmemory.Item
	user    []agentmemory.Item
	added   chan string
	review  chan agentmemory.ReviewDecision
}

func newAgentMemoryServiceStub() *agentMemoryServiceStub {
	now := time.Now()
	return &agentMemoryServiceStub{
		project: []agentmemory.Item{{
			ID: "mem_pending", Scope: agentmemory.Project, Content: "confirm release steps",
			Origin: agentmemory.Automatic, Status: agentmemory.Pending, SessionID: "ses_origin",
			Day: "2026-08-12", CreatedAt: now, UpdatedAt: now,
		}},
		user: []agentmemory.Item{{
			ID: "mem_user", Scope: agentmemory.User, Content: "prefer concise answers",
			Origin: agentmemory.Authored, Status: agentmemory.Active, Pinned: true,
			CreatedAt: now, UpdatedAt: now,
		}},
		added: make(chan string, 1), review: make(chan agentmemory.ReviewDecision, 1),
	}
}

func (service *agentMemoryServiceStub) Items(_ context.Context, target agentmemory.Target) ([]agentmemory.Item, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if target.Scope == agentmemory.User {
		return append([]agentmemory.Item(nil), service.user...), nil
	}
	return append([]agentmemory.Item(nil), service.project...), nil
}

func (service *agentMemoryServiceStub) Review(_ context.Context, id string, decision agentmemory.ReviewDecision) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.project {
		if service.project[index].ID != id {
			continue
		}
		if service.project[index].Status != agentmemory.Pending {
			return errors.New("not pending")
		}
		if decision == agentmemory.Reject {
			service.project = append(service.project[:index], service.project[index+1:]...)
		} else {
			service.project[index].Status = agentmemory.Active
			service.project[index].UpdatedAt = time.Now()
		}
		service.review <- decision
		return nil
	}
	return errors.New("not found")
}

func (service *agentMemoryServiceStub) Update(_ context.Context, patch agentmemory.Patch) (agentmemory.Item, error) {
	if err := patch.Validate(); err != nil {
		return agentmemory.Item{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	for _, items := range []*[]agentmemory.Item{&service.project, &service.user} {
		for index := range *items {
			item := &(*items)[index]
			if item.ID != patch.ID {
				continue
			}
			if patch.Content != nil {
				item.Content = *patch.Content
			}
			if patch.Pinned != nil {
				item.Pinned = *patch.Pinned
			}
			item.UpdatedAt = time.Now()
			return *item, nil
		}
	}
	return agentmemory.Item{}, errors.New("not found")
}

func (service *agentMemoryServiceStub) Delete(_ context.Context, id string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	for _, items := range []*[]agentmemory.Item{&service.project, &service.user} {
		for index := range *items {
			if (*items)[index].ID == id {
				*items = append((*items)[:index], (*items)[index+1:]...)
				return nil
			}
		}
	}
	return errors.New("not found")
}

func (service *agentMemoryServiceStub) Add(_ context.Context, target agentmemory.Target, content string) (agentmemory.Item, error) {
	if err := target.Validate(); err != nil {
		return agentmemory.Item{}, err
	}
	now := time.Now()
	item := agentmemory.Item{
		ID: "mem_added", Scope: target.Scope, Content: content, Origin: agentmemory.Authored,
		Status: agentmemory.Active, CreatedAt: now, UpdatedAt: now,
	}
	service.mu.Lock()
	if target.Scope == agentmemory.User {
		service.user = append(service.user, item)
	} else {
		service.project = append(service.project, item)
	}
	service.mu.Unlock()
	service.added <- content
	return item, nil
}

type knowledgeServiceStub struct {
	mu        sync.Mutex
	content   map[knowledge.Scope]string
	revisions map[knowledge.Scope]string
	saved     chan string
	failed    chan struct{}
	failNext  bool
	blockNext <-chan struct{}
	started   chan string
}

func newKnowledgeServiceStub() *knowledgeServiceStub {
	return &knowledgeServiceStub{
		content: map[knowledge.Scope]string{
			knowledge.WorkingDirectory: "cwd guidance",
			knowledge.ProjectRoot:      "project rules",
			knowledge.Home:             "global preferences",
		},
		revisions: map[knowledge.Scope]string{
			knowledge.WorkingDirectory: "rev-cwd", knowledge.ProjectRoot: "rev-project", knowledge.Home: "rev-home",
		},
		saved: make(chan string, 1), failed: make(chan struct{}, 1),
	}
}

func (service *knowledgeServiceStub) Entries(context.Context, string) ([]knowledge.Entry, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := time.Now()
	return []knowledge.Entry{
		{Scope: knowledge.WorkingDirectory, Content: service.content[knowledge.WorkingDirectory], Revision: service.revisions[knowledge.WorkingDirectory], UpdatedAt: &now},
		{Scope: knowledge.ProjectRoot, Content: service.content[knowledge.ProjectRoot], Revision: service.revisions[knowledge.ProjectRoot], UpdatedAt: &now},
		{Scope: knowledge.Home, Content: service.content[knowledge.Home], Revision: service.revisions[knowledge.Home], UpdatedAt: &now},
	}, nil
}

func (service *knowledgeServiceStub) Document(_ context.Context, target knowledge.Target) (knowledge.Entry, error) {
	if err := target.Validate(); err != nil {
		return knowledge.Entry{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	return knowledge.Entry{Scope: target.Scope, Content: service.content[target.Scope], Revision: service.revisions[target.Scope]}, nil
}

func (service *knowledgeServiceStub) Save(ctx context.Context, update knowledge.Update) (knowledge.Entry, error) {
	if err := update.Validate(); err != nil {
		return knowledge.Entry{}, err
	}
	target, content := update.Target, update.Content
	service.mu.Lock()
	if service.revisions[target.Scope] != update.ExpectedRevision {
		service.mu.Unlock()
		return knowledge.Entry{}, errors.New("revision conflict")
	}
	if service.failNext {
		service.failNext = false
		service.mu.Unlock()
		service.failed <- struct{}{}
		return knowledge.Entry{}, errors.New("write refused")
	}
	block := service.blockNext
	service.blockNext = nil
	service.mu.Unlock()
	if block != nil {
		service.started <- content
		select {
		case <-block:
		case <-ctx.Done():
			return knowledge.Entry{}, context.Cause(ctx)
		}
	}
	service.mu.Lock()
	service.content[target.Scope] = content
	service.revisions[target.Scope] += "+1"
	entry := knowledge.Entry{Scope: target.Scope, Content: content, Revision: service.revisions[target.Scope]}
	service.mu.Unlock()
	service.saved <- content
	return entry, nil
}

func TestAgentMemoryAndKnowledgeReadersShowScopeAndProvenance(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	knowledgeStore := newKnowledgeServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", AgentMemory: memory, Knowledge: knowledgeStore,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/memory project")
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · project")
	host.Shows(t, "session  ses_origin")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/knowledge")
	host.Press(input.Enter)
	host.Shows(t, "LYRA.md knowledge")
	host.Shows(t, "global preferences")
	stop()
}

func TestAgentMemoryMultilineAddSurvivesResize(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", AgentMemory: memory,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/memory-add user")
	host.Press(input.Enter)
	host.Shows(t, "Add user memory")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory editor did not survive a minimal viewport")
	}
	host.Shows(t, "Add user memory")
	host.Type("first line")
	host.Press(input.Enter)
	host.Type("second line")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, memory.added, "agent memory add"); got != "first line\nsecond line" {
		t.Fatalf("added content = %q", got)
	}
	host.Shows(t, "Agent memory · user")
	host.Shows(t, "first line")
	stop()
}

func TestPendingAgentMemoryReviewRequiresResizeSafeConfirmation(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", AgentMemory: memory,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/memory-approve project mem_pending")
	host.Press(input.Enter)
	host.Shows(t, "Approve agent memory")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory confirmation did not survive a minimal viewport")
	}
	host.Shows(t, "Approve agent memory")
	host.Press(input.Down)
	host.Press(input.Enter)
	if got := awaitValue(t, memory.review, "agent memory review"); got != agentmemory.Approve {
		t.Fatalf("review = %q", got)
	}
	host.Shows(t, "Agent memory · project")
	host.Shows(t, "active")
	stop()
}

func TestAgentMemoryEditPinAndDeleteRoundTripThroughAuthoritativeReads(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", AgentMemory: memory,
	})
	host.Shows(t, "Ask lyra")

	host.Type("/memory-edit user mem_user")
	host.Press(input.Enter)
	host.Shows(t, "Edit agent memory · mem_user")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("prefer explicit answers")
	host.Press(input.Enter)
	host.Type("with evidence")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory editor did not survive a minimal viewport")
	}
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "Agent memory · user")
	host.Shows(t, "prefer explicit answers")
	items, err := memory.Items(t.Context(), agentmemory.Target{Scope: agentmemory.User})
	if err != nil || len(items) != 1 || items[0].Content != "prefer explicit answers\nwith evidence" {
		t.Fatalf("edited user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/memory-unpin user mem_user")
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · user")
	items, err = memory.Items(t.Context(), agentmemory.Target{Scope: agentmemory.User})
	if err != nil || len(items) != 1 || items[0].Pinned {
		t.Fatalf("unpinned user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/memory-pin user mem_user")
	host.Press(input.Enter)
	host.Shows(t, "pinned")
	items, err = memory.Items(t.Context(), agentmemory.Target{Scope: agentmemory.User})
	if err != nil || len(items) != 1 || !items[0].Pinned {
		t.Fatalf("pinned user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/memory-delete user mem_user")
	host.Press(input.Enter)
	host.Shows(t, "Delete agent memory")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory deletion did not survive a minimal viewport")
	}
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "No active or pending memory")
	items, err = memory.Items(t.Context(), agentmemory.Target{Scope: agentmemory.User})
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted user memory = (%+v, %v)", items, err)
	}
	stop()
}

func TestKnowledgeReadUsesTheRequestedScope(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Knowledge: knowledgeStore,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/knowledge-read home")
	host.Press(input.Enter)
	host.Shows(t, "LYRA.md · home")
	host.Shows(t, "global preferences")
	stop()
}

func TestKnowledgeChangeConvergesTheExactOpenScope(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.KnowledgeChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Knowledge: knowledgeStore, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	subscription := awaitValue(t, source.subscription, "knowledge change subscription")
	if !slices.Equal(subscription.Topics, []changefeed.Topic{changefeed.KnowledgeChanged}) {
		t.Fatalf("knowledge subscription = %v", subscription.Topics)
	}
	host.Type("/knowledge-read home")
	host.Press(input.Enter)
	host.Shows(t, "global preferences")

	knowledgeStore.mu.Lock()
	knowledgeStore.content[knowledge.Home] = "preferences from runtime change"
	knowledgeStore.revisions[knowledge.Home] = "rev-home+external"
	knowledgeStore.mu.Unlock()
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.KnowledgeChanged), Sequence: 1}
	awaitValue(t, source.applied, "knowledge invalidation")
	host.Shows(t, "preferences from runtime change")
	host.Hides(t, "cwd guidance")
	stop()
}

func TestKnowledgeResyncConvergesTheExactOpenScope(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.KnowledgeChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Knowledge: knowledgeStore, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "knowledge resync subscription")
	host.Type("/knowledge-read home")
	host.Press(input.Enter)
	host.Shows(t, "global preferences")

	knowledgeStore.mu.Lock()
	knowledgeStore.content[knowledge.Home] = "preferences from scoped resync"
	knowledgeStore.revisions[knowledge.Home] = "rev-home+resync"
	knowledgeStore.mu.Unlock()
	source.events <- changefeed.Event{
		Type: changefeed.Resync, Sequence: 1, Topics: []changefeed.Topic{changefeed.KnowledgeChanged},
	}
	awaitValue(t, source.applied, "knowledge resync")
	host.Shows(t, "preferences from scoped resync")
	stop()
}

func TestKnowledgeEditorPreservesMultilineContentAcrossResize(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Knowledge: knowledgeStore,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit LYRA.md · projectRoot")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("knowledge editor did not survive a minimal viewport")
	}
	host.Shows(t, "Edit LYRA.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("line one")
	host.Press(input.Enter)
	host.Type("line two")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "knowledge save"); got != "line one\nline two" {
		t.Fatalf("saved content = %q", got)
	}
	host.Shows(t, "LYRA.md · projectRoot")
	host.Shows(t, "line one")
	stop()
}

func TestKnowledgeEditorRetainsDraftWhenRuntimeSaveFails(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	knowledgeStore.failNext = true
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Knowledge: knowledgeStore,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit LYRA.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("unsaved draft")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	awaitValue(t, knowledgeStore.failed, "failed knowledge save")
	host.Shows(t, "Edit LYRA.md · projectRoot")
	host.Shows(t, "write refused")
	host.Shows(t, "unsaved draft")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("failed knowledge editor did not survive a minimal viewport")
	}
	host.Shows(t, "unsaved draft")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "retried knowledge save"); got != "unsaved draft" {
		t.Fatalf("retried content = %q", got)
	}
	host.Shows(t, "LYRA.md · projectRoot")
	stop()
}

func TestKnowledgeEditorDoesNotLoseEditsMadeWhileSaving(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	release := make(chan struct{})
	knowledgeStore.blockNext = release
	knowledgeStore.started = make(chan string, 1)
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Knowledge: knowledgeStore,
	})
	host.Shows(t, "Ask lyra")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit LYRA.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("first draft")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.started, "knowledge save start"); got != "first draft" {
		t.Fatalf("first submitted content = %q", got)
	}
	host.Type(" with newer edits")
	host.Shows(t, "first draft with newer edits")
	close(release)
	if got := awaitValue(t, knowledgeStore.saved, "first knowledge save"); got != "first draft" {
		t.Fatalf("first saved content = %q", got)
	}
	host.Shows(t, "Saved. New edits remain unsaved.")
	host.Shows(t, "first draft with newer edits")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "second knowledge save"); got != "first draft with newer edits" {
		t.Fatalf("second saved content = %q", got)
	}
	host.Shows(t, "LYRA.md · projectRoot")
	stop()
}

func TestKnowledgeEditorCanRetryAfterSameSessionProjectionCancelsItsSave(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	knowledgeStore.blockNext = make(chan struct{})
	knowledgeStore.started = make(chan string, 1)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	backend := mock.New()
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", Knowledge: knowledgeStore, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit LYRA.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("draft survives projection refresh")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.started, "blocked knowledge save"); got != "draft survives projection refresh" {
		t.Fatalf("blocked save content = %q", got)
	}
	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}

	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	host.Shows(t, "Save interrupted by session refresh. Draft remains unsaved.")
	host.Shows(t, "draft survives projection refresh")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "retried knowledge save"); got != "draft survives projection refresh" {
		t.Fatalf("retried save content = %q", got)
	}
	host.Shows(t, "LYRA.md · projectRoot")
	stop()
}
