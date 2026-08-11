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
	FilesChanged      Topic = "files.changed"
	SkillsChanged     Topic = "skills.changed"
	MCPChanged        Topic = "mcp.changed"
	SchedulesChanged  Topic = "schedules.changed"
	SessionsChanged   Topic = "sessions.changed"
	RunsChanged       Topic = "runs.changed"
	StateChanged      Topic = "state.changed"
	GoalsChanged      Topic = "goals.changed"
	InterruptsChanged Topic = "interrupts.changed"
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
		StateChanged,
		GoalsChanged,
		InterruptsChanged,
	}
}

func (topic Topic) Valid() bool {
	return slices.Contains(Topics(), topic)
}

type EventType string

const Resync EventType = "resync"

type Watch struct {
	ID        string
	Workspace string
}

func (watch Watch) Validate() error {
	if strings.TrimSpace(watch.ID) == "" || strings.TrimSpace(watch.Workspace) == "" {
		return errors.New("change watch requires id and workspace")
	}
	return nil
}

type Subscription struct {
	Topics  []Topic
	Watches []Watch
}

func (subscription Subscription) Validate() error {
	if len(subscription.Topics) == 0 {
		return errors.New("change subscription has no topics")
	}
	seen := make(map[Topic]struct{}, len(subscription.Topics))
	for _, topic := range subscription.Topics {
		if !topic.Valid() {
			return fmt.Errorf("change subscription topic %q is invalid", topic)
		}
		if _, duplicate := seen[topic]; duplicate {
			return fmt.Errorf("change subscription repeats topic %q", topic)
		}
		seen[topic] = struct{}{}
	}
	if len(subscription.Watches) > 0 && !slices.Contains(subscription.Topics, FilesChanged) {
		return errors.New("file watches require the files.changed topic")
	}
	watchIDs := make(map[string]struct{}, len(subscription.Watches))
	for _, watch := range subscription.Watches {
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
	StateKey    string
	Topics      []Topic
	WatchIDs    []string
}

func (event Event) Validate() error {
	if event.Sequence == 0 {
		return errors.New("change event sequence is zero")
	}
	if event.Type == Resync {
		if len(event.Topics) == 0 && len(event.WatchIDs) == 0 {
			return errors.New("resync event has no affected scope")
		}
		return nil
	}
	topic := Topic(event.Type)
	if !topic.Valid() {
		return fmt.Errorf("change event type %q is invalid", event.Type)
	}
	if topic == FilesChanged {
		if strings.TrimSpace(event.WatchID) == "" || strings.TrimSpace(event.Workspace) == "" || len(event.Paths) == 0 {
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
