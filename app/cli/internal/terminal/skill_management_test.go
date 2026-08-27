package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/agent/mock"
	"github.com/Tangerg/scope/app/cli/internal/changefeed"
	"github.com/Tangerg/scope/app/cli/internal/skills"
)

const terminalSkillRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestResolveSkillProposalRequiresRevisionWhenNamesAreNotUnique(t *testing.T) {
	first := skills.Proposal{Name: "shared", Scope: skills.UserScope, Revision: terminalSkillRevision}
	second := skills.Proposal{Name: "shared", Scope: skills.UserScope, Revision: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"}
	proposals := []skills.Proposal{first, second}
	if _, err := resolveSkillProposal(proposals, "user/shared"); err == nil {
		t.Fatal("ambiguous proposal name was accepted")
	}
	resolved, err := resolveSkillProposal(proposals, second.Key())
	if err != nil || resolved.Revision != second.Revision {
		t.Fatalf("resolve exact proposal = (%+v, %v)", resolved, err)
	}
}

type skillServiceStub struct {
	mu              sync.Mutex
	discovered      []skills.Discovered
	managed         []skills.Managed
	proposals       []skills.Proposal
	reads           atomic.Int32
	decisions       chan skillDecision
	ignoreLifecycle bool
	ignoreDecision  bool
}

type skillDecision struct {
	approve   bool
	reference skills.ProposalReference
}

type blockingSkillArchiveService struct {
	skills.Service
	started   chan string
	release   chan struct{}
	canceled  chan struct{}
	committed chan error
}

func (b *blockingSkillArchiveService) Archive(ctx context.Context, name string) error {
	b.started <- name
	select {
	case <-b.release:
		err := b.Service.Archive(ctx, name)
		b.committed <- err
		return err
	case <-ctx.Done():
		close(b.canceled)
		return context.Cause(ctx)
	}
}

func newSkillServiceStub() *skillServiceStub {
	return &skillServiceStub{
		discovered: []skills.Discovered{{Name: "release-checks", Description: "Release safely", Scope: skills.ProjectScope}},
		managed:    []skills.Managed{{Name: "review", Description: "Review code", Lifecycle: skills.Active}},
		proposals: []skills.Proposal{
			{Name: "release-checks", Revision: terminalSkillRevision, Scope: skills.UserScope, Description: "Release safely", Instructions: "Run every release gate.", Origin: skills.Requested},
			{Name: "cleanup", Revision: terminalSkillRevision, Scope: skills.ProjectScope, Description: "Clean generated files", Instructions: "Remove only generated output.", Origin: skills.Mined},
		},
		decisions: make(chan skillDecision, 2),
	}
}

func (s *skillServiceStub) Discover(context.Context, string) ([]skills.Discovered, error) {
	s.reads.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]skills.Discovered(nil), s.discovered...), nil
}

func (s *skillServiceStub) Managed(context.Context) ([]skills.Managed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]skills.Managed(nil), s.managed...), nil
}

func (s *skillServiceStub) Proposals(context.Context, string) ([]skills.Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]skills.Proposal(nil), s.proposals...), nil
}

func (s *skillServiceStub) Archive(_ context.Context, name string) error {
	return s.setLifecycle(name, skills.Archived)
}

func (s *skillServiceStub) Restore(_ context.Context, name string) error {
	return s.setLifecycle(name, skills.Active)
}

func (s *skillServiceStub) setLifecycle(name string, lifecycle skills.Lifecycle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.managed {
		if s.managed[index].Name == name {
			if !s.ignoreLifecycle {
				s.managed[index].Lifecycle = lifecycle
			}
			return nil
		}
	}
	return errors.New("skill not found")
}

func (s *skillServiceStub) Approve(_ context.Context, reference skills.ProposalReference) error {
	return s.decide(reference, true)
}

func (s *skillServiceStub) Reject(_ context.Context, reference skills.ProposalReference) error {
	return s.decide(reference, false)
}

func (s *skillServiceStub) decide(reference skills.ProposalReference, approve bool) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, proposal := range s.proposals {
		if proposal.Name == reference.Name && proposal.Scope == reference.Scope && proposal.Revision == reference.Revision {
			if !s.ignoreDecision {
				s.proposals = append(s.proposals[:index], s.proposals[index+1:]...)
			}
			s.decisions <- skillDecision{approve: approve, reference: reference}
			return nil
		}
	}
	return errors.New("proposal changed")
}

func TestSkillCatalogLifecycleAndProposalReviewCommands(t *testing.T) {
	service := newSkillServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Skills: service})
	host.Shows(t, "Ask lyra")
	host.Type("/skills")
	host.Press(input.Enter)
	host.Shows(t, "project/release-checks")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")

	host.Type("/skill-library")
	host.Press(input.Enter)
	host.Shows(t, "review  active")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/skill-archive review")
	host.Press(input.Enter)
	host.Shows(t, "archiving skill complete · review")
	host.Type("/skill-restore review")
	host.Press(input.Enter)
	host.Shows(t, "restoring skill complete · review")

	host.Type("/skill-proposals")
	host.Press(input.Enter)
	host.Shows(t, "Run every release gate.")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/skill-approve user/release-checks")
	host.Press(input.Enter)
	host.Shows(t, "Approve Skill proposal")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("skill proposal confirmation did not survive a minimal viewport")
	}
	host.Shows(t, "Approve Skill proposal")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "approving skill proposal complete · user/release-checks")
	approved := <-service.decisions
	if !approved.approve || approved.reference.Workspace == "" || approved.reference.Revision != terminalSkillRevision {
		t.Fatalf("approved decision = %+v", approved)
	}

	host.Type("/skill-reject project/cleanup")
	host.Press(input.Enter)
	host.Shows(t, "Reject Skill proposal")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "rejecting skill proposal complete · project/cleanup")
	rejected := <-service.decisions
	if rejected.approve || rejected.reference.Name != "cleanup" {
		t.Fatalf("rejected decision = %+v", rejected)
	}
	stop()
}

func TestSkillLifecycleDoesNotReportSuccessWhenManagedCatalogIsUnchanged(t *testing.T) {
	service := newSkillServiceStub()
	service.ignoreLifecycle = true
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Skills: service})
	host.Shows(t, "Ask lyra")
	host.Type("/skill-archive review")
	host.Press(input.Enter)
	host.Shows(t, "archiving skill failed: verify skill lifecycle")
	host.Hides(t, "archiving skill complete")
	stop()
}

func TestSkillProposalDoesNotReportSuccessWhenReviewedRevisionRemainsPending(t *testing.T) {
	service := newSkillServiceStub()
	service.ignoreDecision = true
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Skills: service})
	host.Shows(t, "Ask lyra")
	host.Type("/skill-approve user/release-checks")
	host.Press(input.Enter)
	host.Shows(t, "Approve Skill proposal")
	host.Press(input.Down)
	host.Press(input.Enter)
	decision := awaitValue(t, service.decisions, "ignored skill proposal decision")
	if !decision.approve {
		t.Fatal("skill proposal approval was sent as rejection")
	}
	host.Shows(t, "approving skill proposal failed: verify skill proposal decision")
	host.Hides(t, "approving skill proposal complete")
	stop()
}

func TestSkillsChangedRefetchesOnlyAnOpenSkillProjection(t *testing.T) {
	service := newSkillServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.SkillsChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Skills: service, Changes: source})
	host.Shows(t, "Ask lyra")
	subscription := awaitValue(t, source.subscription, "skill invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != changefeed.SkillsChanged {
		t.Fatalf("skill subscription = %+v", subscription)
	}
	host.Type("/skills")
	host.Press(input.Enter)
	host.Shows(t, "Release safely")
	baseline := service.reads.Load()
	service.mu.Lock()
	service.discovered[0].Description = "Release with verified artifacts"
	service.mu.Unlock()
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.SkillsChanged), Sequence: 1, Names: []string{"release-checks"}}
	awaitSignal(t, source.applied, "skills.changed delivery")
	host.Shows(t, "Release with verified artifacts")
	if service.reads.Load() <= baseline {
		t.Fatal("skills.changed did not refetch the open Skill projection")
	}
	stop()
}

func TestSkillLifecycleMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	backend := mock.New()
	base := newSkillServiceStub()
	service := &blockingSkillArchiveService{
		Service: base, started: make(chan string, 1), release: make(chan struct{}), canceled: make(chan struct{}),
		committed: make(chan error, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", Skills: service, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/skill-archive review")
	host.Press(input.Enter)
	if got := awaitValue(t, service.started, "skill archive mutation"); got != "review" {
		t.Fatalf("archived skill = %q", got)
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
	select {
	case <-service.canceled:
		t.Fatal("session projection replacement canceled the skill mutation")
	default:
	}
	release()
	if err := awaitValue(t, service.committed, "committed skill archive"); err != nil {
		t.Fatal(err)
	}
	managed, err := base.Managed(t.Context())
	if err != nil || len(managed) != 1 || managed[0].Lifecycle != skills.Archived {
		t.Fatalf("managed skills after archive = (%+v, %v)", managed, err)
	}
	stop()
}
