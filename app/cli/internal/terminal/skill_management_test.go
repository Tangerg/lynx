package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/skills"
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
	mu         sync.Mutex
	discovered []skills.Discovered
	managed    []skills.Managed
	proposals  []skills.Proposal
	reads      atomic.Int32
	decisions  chan skillDecision
}

type skillDecision struct {
	approve   bool
	reference skills.ProposalReference
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

func (service *skillServiceStub) Discover(context.Context, string) ([]skills.Discovered, error) {
	service.reads.Add(1)
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]skills.Discovered(nil), service.discovered...), nil
}

func (service *skillServiceStub) Managed(context.Context) ([]skills.Managed, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]skills.Managed(nil), service.managed...), nil
}

func (service *skillServiceStub) Proposals(context.Context, string) ([]skills.Proposal, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]skills.Proposal(nil), service.proposals...), nil
}

func (service *skillServiceStub) Archive(_ context.Context, name string) error {
	return service.setLifecycle(name, skills.Archived)
}

func (service *skillServiceStub) Restore(_ context.Context, name string) error {
	return service.setLifecycle(name, skills.Active)
}

func (service *skillServiceStub) setLifecycle(name string, lifecycle skills.Lifecycle) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.managed {
		if service.managed[index].Name == name {
			service.managed[index].Lifecycle = lifecycle
			return nil
		}
	}
	return errors.New("skill not found")
}

func (service *skillServiceStub) Approve(_ context.Context, reference skills.ProposalReference) error {
	return service.decide(reference, true)
}

func (service *skillServiceStub) Reject(_ context.Context, reference skills.ProposalReference) error {
	return service.decide(reference, false)
}

func (service *skillServiceStub) decide(reference skills.ProposalReference, approve bool) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	for index, proposal := range service.proposals {
		if proposal.Name == reference.Name && proposal.Scope == reference.Scope && proposal.Revision == reference.Revision {
			service.proposals = append(service.proposals[:index], service.proposals[index+1:]...)
			service.decisions <- skillDecision{approve: approve, reference: reference}
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
