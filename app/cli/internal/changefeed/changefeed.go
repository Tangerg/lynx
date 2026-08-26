// Package changefeed models runtime-wide invalidations. Events deliberately
// carry identities, not duplicated resource state; consumers refetch through
// the authoritative bounded-context query port.
package changefeed

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
)

type Topic string

const (
	FilesChanged       Topic = "files.changed"
	SkillsChanged      Topic = "skills.changed"
	MCPChanged         Topic = "mcp.changed"
	SchedulesChanged   Topic = "schedules.changed"
	SessionsChanged    Topic = "sessions.changed"
	RunsChanged        Topic = "runs.changed"
	PlanChanged        Topic = "plan.changed"
	GoalsChanged       Topic = "goals.changed"
	InterruptsChanged  Topic = "interrupts.changed"
	KnowledgeChanged   Topic = "knowledge.changed"
	HooksChanged       Topic = "hooks.changed"
	ModelsChanged      Topic = "models.changed"
	ApprovalsChanged   Topic = "approvals.changed"
	AgentMemoryChanged Topic = "agentMemory.changed"
)

// Topics returns the complete change vocabulary understood by this client.
// Callers own the returned slice, so subscription policy cannot mutate the
// package's inventory.
func Topics() []Topic {
	return []Topic{
		FilesChanged,
		SkillsChanged,
		MCPChanged,
		SchedulesChanged,
		SessionsChanged,
		RunsChanged,
		PlanChanged,
		GoalsChanged,
		InterruptsChanged,
		KnowledgeChanged,
		HooksChanged,
		ModelsChanged,
		ApprovalsChanged,
		AgentMemoryChanged,
	}
}

func (t Topic) Valid() bool {
	return slices.Contains(Topics(), t)
}

type EventType string

const Resync EventType = "resync"

type Watch struct {
	ID        string
	Workspace string
}

func (w Watch) Validate() error {
	if strings.TrimSpace(w.ID) == "" || strings.TrimSpace(w.Workspace) == "" {
		return errors.New("change watch requires id and workspace")
	}
	return nil
}

type Subscription struct {
	Topics  []Topic
	Watches []Watch
}

// SubscriptionLimits are the delivery constraints negotiated with the
// runtime. A zero value means the transport did not advertise a constraint.
// Partition keeps those transport details out of change consumers while
// preserving every requested topic and workspace-observation scope.
type SubscriptionLimits struct {
	MaxTopics  int
	MaxWatches int
}

func (s SubscriptionLimits) Partition(subscription Subscription) ([]Subscription, error) {
	if err := subscription.Validate(); err != nil {
		return nil, err
	}
	if s.MaxTopics < 0 || s.MaxWatches < 0 {
		return nil, errors.New("change subscription limits cannot be negative")
	}
	topicLimit := len(subscription.Topics)
	if s.MaxTopics > 0 {
		topicLimit = min(s.MaxTopics, topicLimit)
	}
	watchLimit := len(subscription.Watches)
	if s.MaxWatches > 0 {
		watchLimit = min(s.MaxWatches, watchLimit)
	}
	if len(subscription.Topics) <= topicLimit && len(subscription.Watches) <= watchLimit {
		return []Subscription{cloneSubscription(subscription)}, nil
	}
	if len(subscription.Watches) > 0 {
		workspaceTopics := workspaceObservedTopics(subscription.Topics)
		if len(workspaceTopics) > 0 {
			return partitionWorkspaceObservation(subscription, workspaceTopics, topicLimit, watchLimit)
		}
	}

	partitions := partitionTopics(subscription.Topics, topicLimit)
	if len(subscription.Watches) == 0 {
		return partitions, nil
	}

	filePartition := slices.Index(subscription.Topics, FilesChanged) / topicLimit
	remainingWatches := subscription.Watches
	count := min(watchLimit, len(remainingWatches))
	partitions[filePartition].Watches = slices.Clone(remainingWatches[:count])
	remainingWatches = remainingWatches[count:]
	for len(remainingWatches) > 0 {
		count = min(watchLimit, len(remainingWatches))
		partitions = append(partitions, Subscription{
			Topics:  []Topic{FilesChanged},
			Watches: slices.Clone(remainingWatches[:count]),
		})
		remainingWatches = remainingWatches[count:]
	}
	return partitions, nil
}

// workspaceObservedTopics are global without a watch, but observe the named
// workspace's externally-authored files when a watch accompanies them. The
// runtime wire contract uses FilesChanged as the anchor that makes a watch
// legal, so a partitioner must keep that anchor and scope beside each topic.
func workspaceObservedTopics(topics []Topic) []Topic {
	observed := make([]Topic, 0, 2)
	for _, topic := range topics {
		if topic == KnowledgeChanged || topic == HooksChanged {
			observed = append(observed, topic)
		}
	}
	return observed
}

func partitionWorkspaceObservation(
	subscription Subscription,
	workspaceTopics []Topic,
	topicLimit int,
	watchLimit int,
) ([]Subscription, error) {
	if topicLimit < 2 {
		return nil, errors.New("change subscription topic limit cannot preserve workspace observation")
	}
	workspaceTopicPartitions := partitionTopics(workspaceTopics, topicLimit-1)
	watchPartitions := partitionWatches(subscription.Watches, watchLimit)
	partitions := make([]Subscription, 0, len(workspaceTopicPartitions)*len(watchPartitions))
	for _, watches := range watchPartitions {
		for _, topics := range workspaceTopicPartitions {
			partitions = append(partitions, Subscription{
				Topics:  append([]Topic{FilesChanged}, topics.Topics...),
				Watches: slices.Clone(watches),
			})
		}
	}

	unscoped := make([]Topic, 0, len(subscription.Topics))
	for _, topic := range subscription.Topics {
		if topic != FilesChanged && !slices.Contains(workspaceTopics, topic) {
			unscoped = append(unscoped, topic)
		}
	}
	return append(partitions, partitionTopics(unscoped, topicLimit)...), nil
}

func partitionTopics(topics []Topic, limit int) []Subscription {
	partitions := make([]Subscription, 0, (len(topics)+limit-1)/limit)
	for remaining := topics; len(remaining) > 0; {
		count := min(limit, len(remaining))
		partitions = append(partitions, Subscription{Topics: slices.Clone(remaining[:count])})
		remaining = remaining[count:]
	}
	return partitions
}

func partitionWatches(watches []Watch, limit int) [][]Watch {
	partitions := make([][]Watch, 0, (len(watches)+limit-1)/limit)
	for remaining := watches; len(remaining) > 0; {
		count := min(limit, len(remaining))
		partitions = append(partitions, slices.Clone(remaining[:count]))
		remaining = remaining[count:]
	}
	return partitions
}

func cloneSubscription(subscription Subscription) Subscription {
	return Subscription{
		Topics:  slices.Clone(subscription.Topics),
		Watches: slices.Clone(subscription.Watches),
	}
}

func (s Subscription) Validate() error {
	if len(s.Topics) == 0 {
		return errors.New("change subscription has no topics")
	}
	seen := make(map[Topic]struct{}, len(s.Topics))
	for _, topic := range s.Topics {
		if !topic.Valid() {
			return fmt.Errorf("change subscription topic %q is invalid", topic)
		}
		if _, duplicate := seen[topic]; duplicate {
			return fmt.Errorf("change subscription repeats topic %q", topic)
		}
		seen[topic] = struct{}{}
	}
	if len(s.Watches) > 0 && !slices.Contains(s.Topics, FilesChanged) {
		return errors.New("file watches require the files.changed topic")
	}
	watchIDs := make(map[string]struct{}, len(s.Watches))
	for _, watch := range s.Watches {
		if err := watch.Validate(); err != nil {
			return err
		}
		if _, duplicate := watchIDs[watch.ID]; duplicate {
			return fmt.Errorf("change subscription repeats watch %q", watch.ID)
		}
		watchIDs[watch.ID] = struct{}{}
	}
	return nil
}

// ValidateEvent checks both the event shape and the delivery scope owned by
// this subscription. Runtime events are invalidations, so accepting a frame
// for an undeclared topic or watch can make an unrelated local projection look
// authoritative.
func (s Subscription) ValidateEvent(event Event) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if event.Type != Resync {
		topic := Topic(event.Type)
		if !slices.Contains(s.Topics, topic) {
			return fmt.Errorf("change event topic %q is outside the subscription", topic)
		}
		if topic == FilesChanged && event.WatchID != "" {
			watchIndex := slices.IndexFunc(s.Watches, func(watch Watch) bool {
				return watch.ID == event.WatchID
			})
			if watchIndex < 0 {
				return fmt.Errorf("file change watch %q is outside the subscription", event.WatchID)
			}
			if event.Workspace != "" && event.Workspace != s.Watches[watchIndex].Workspace {
				return fmt.Errorf("file change watch %q names another workspace", event.WatchID)
			}
		}
		return nil
	}

	for _, topic := range event.Topics {
		if !slices.Contains(s.Topics, topic) {
			return fmt.Errorf("resync topic %q is outside the subscription", topic)
		}
	}
	if len(event.WatchIDs) > 0 && !slices.Contains(event.Topics, FilesChanged) {
		return errors.New("resync watch scope requires the files.changed topic")
	}
	for _, watchID := range event.WatchIDs {
		if !slices.ContainsFunc(s.Watches, func(watch Watch) bool { return watch.ID == watchID }) {
			return fmt.Errorf("resync watch %q is outside the subscription", watchID)
		}
	}
	return nil
}

type Event struct {
	Type        EventType
	Sequence    uint64
	WatchID     string
	Workspace   string
	Paths       []string
	Names       []string
	ServerIDs   []string
	ScheduleIDs []string
	SessionIDs  []string
	RunIDs      []string
	Topics      []Topic
	WatchIDs    []string
}

func (e Event) Validate() error {
	if e.Sequence == 0 {
		return errors.New("change event sequence is zero")
	}
	if e.Type == Resync {
		if len(e.Topics) == 0 && len(e.WatchIDs) == 0 {
			return errors.New("resync event has no affected scope")
		}
		return nil
	}
	topic := Topic(e.Type)
	if !topic.Valid() {
		return fmt.Errorf("change event type %q is invalid", e.Type)
	}
	if topic == FilesChanged {
		// Tool writes are broad invalidations and intentionally carry no watch ID.
		// Watch-produced signals may add WatchID and Workspace so consumers can
		// narrow the authoritative read, but neither is required by the protocol.
		if len(e.Paths) == 0 {
			return errors.New("file change event is incomplete")
		}
	}
	return nil
}

type EventStream = iter.Seq2[Event, error]

type Source interface {
	Supports(Topic) bool
	Subscribe(context.Context, Subscription) (EventStream, error)
}
