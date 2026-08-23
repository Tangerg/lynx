// Package compactionflow owns post-Run model-context compaction. The complete
// conversation journal remains immutable for export, rollback, and audit; a
// durable projection records which prefix was summarized for future model
// requests.
package compactionflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	messageTrigger  = 80
	byteTrigger     = 512 << 10
	keepRecent      = 24
	jobCapacity     = 64
	maxSummaryBytes = 64 << 10
)

type Entry struct {
	Ordinal int
	Body    []byte
}

type Candidate struct {
	SessionID, RunID, Workspace, Provider, Model string
	Entries                                      []Entry
	LatestThrough                                int
	MaximumOrdinal                               int
}

type Write struct {
	ID, SessionID, RunID                                          string
	ThroughOrdinal, ExpectedLatestThrough, ExpectedMaximumOrdinal int
	SummaryBody                                                   []byte
	MessagesBefore, MessagesAfter                                 int
	CreatedAt                                                     time.Time
}

type Recovery struct {
	SessionID string
	RunID     string
	Workspace string
}

type Store interface {
	CompactionCandidate(context.Context, string, string, string) (Candidate, error)
	CompactionRecoveries(context.Context) ([]Recovery, error)
	CommitCompaction(context.Context, Write) (bool, error)
}

type Models interface {
	Summarize(context.Context, string, string, []chat.Message, []lifecyclehook.Context) (string, error)
}

type Hooks interface {
	Evaluate(context.Context, lifecyclehook.Invocation) (lifecyclehook.Decision, error)
}

type IDs interface{ New(string) (string, error) }
type Publisher interface{ Publish(protocol.RuntimeEvent) }

type Config struct {
	Store    Store
	Models   Models
	Hooks    Hooks
	IDs      IDs
	Events   Publisher
	Lifetime context.Context
	Logger   *slog.Logger
	Clock    func() time.Time
}

type Service struct {
	store     Store
	models    Models
	hooks     Hooks
	ids       IDs
	events    Publisher
	ctx       context.Context
	cancel    context.CancelFunc
	jobs      chan job
	logger    *slog.Logger
	now       func() time.Time
	mu        sync.Mutex
	recovered bool
	closed    bool
	tasks     sync.WaitGroup
	once      sync.Once
}

type job struct{ sessionID, runID, workspace string }

func New(config Config) (*Service, error) {
	if config.Store == nil || config.Models == nil || config.Hooks == nil || config.IDs == nil || config.Events == nil || config.Lifetime == nil {
		return nil, errors.New("compactionflow: store, models, hooks, ids, events, and lifetime are required")
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	ctx, cancel := context.WithCancel(config.Lifetime)
	service := &Service{store: config.Store, models: config.Models, hooks: config.Hooks, ids: config.IDs, events: config.Events, ctx: ctx, cancel: cancel, jobs: make(chan job, jobCapacity), logger: logger, now: clock}
	service.tasks.Add(1)
	go service.work()
	return service, nil
}

func (service *Service) RunSettled(sessionID, runID, workspace string) {
	if service == nil {
		return
	}
	select {
	case service.jobs <- job{sessionID: sessionID, runID: runID, workspace: workspace}:
	case <-service.ctx.Done():
	default:
		service.logger.Warn("compaction maintenance queue is full", "session_id", sessionID, "run_id", runID)
	}
}

func (service *Service) Recover(ctx context.Context) error {
	if service == nil {
		return errors.New("compactionflow: service is required")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return errors.New("compactionflow: service is closed")
	}
	if service.recovered {
		return errors.New("compactionflow: Recover may be called only once")
	}
	values, err := service.store.CompactionRecoveries(ctx)
	if err != nil {
		return fmt.Errorf("compactionflow: list recovery candidates: %w", err)
	}
	service.recovered = true
	service.tasks.Add(1)
	go func() {
		defer service.tasks.Done()
		for _, value := range values {
			select {
			case service.jobs <- job{sessionID: value.SessionID, runID: value.RunID, workspace: value.Workspace}:
			case <-service.ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (service *Service) work() {
	defer service.tasks.Done()
	for {
		select {
		case value := <-service.jobs:
			if err := service.compact(service.ctx, value); err != nil && service.ctx.Err() == nil {
				service.logger.Warn("conversation compaction did not complete", "session_id", value.sessionID, "run_id", value.runID, "error", err)
			}
		case <-service.ctx.Done():
			return
		}
	}
}

func (service *Service) compact(ctx context.Context, value job) error {
	candidate, err := service.store.CompactionCandidate(ctx, value.sessionID, value.runID, value.workspace)
	if err != nil {
		return err
	}
	if len(candidate.Entries) <= keepRecent {
		return nil
	}
	size := 0
	messages := make([]chat.Message, len(candidate.Entries))
	for index, entry := range candidate.Entries {
		size += len(entry.Body)
		if err := json.Unmarshal(entry.Body, &messages[index]); err != nil {
			return fmt.Errorf("compactionflow: decode message %d: %w", entry.Ordinal, err)
		}
		if err := messages[index].Validate(); err != nil {
			return err
		}
	}
	if len(messages) < messageTrigger && size < byteTrigger {
		return nil
	}
	cutoff := summaryCutoff(messages)
	if cutoff <= 0 || cutoff >= len(messages) {
		return nil
	}
	through := candidate.Entries[cutoff-1].Ordinal
	decision, err := service.hooks.Evaluate(ctx, lifecyclehook.Invocation{Event: lifecyclehook.PreCompact, SessionID: value.sessionID, RunID: value.runID, Workspace: value.workspace, Reason: fmt.Sprintf("summarizing %d of %d model-context messages", cutoff, len(messages))})
	if err != nil {
		return fmt.Errorf("compactionflow: evaluate PreCompact: %w", err)
	}
	if decision.Denied() {
		return nil
	}
	summary, err := service.models.Summarize(ctx, candidate.Provider, candidate.Model, messages[:cutoff], decision.Contexts)
	if err != nil {
		return err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return errors.New("compactionflow: model returned an empty summary")
	}
	if len(summary) > maxSummaryBytes {
		return errors.New("compactionflow: model summary exceeds the 64 KiB limit")
	}
	message := chat.NewSystemMessage("[Earlier conversation summary]\n" + summary)
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	id, err := service.ids.New("itm_")
	if err != nil {
		return err
	}
	committed, err := service.store.CommitCompaction(ctx, Write{ID: id, SessionID: value.sessionID, RunID: value.runID, ThroughOrdinal: through, ExpectedLatestThrough: candidate.LatestThrough, ExpectedMaximumOrdinal: candidate.MaximumOrdinal, SummaryBody: body, MessagesBefore: len(messages), MessagesAfter: 1 + len(messages) - cutoff, CreatedAt: service.now().UTC()})
	if err != nil {
		return err
	}
	if committed {
		service.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, SessionIDs: []string{value.sessionID}})
	}
	return nil
}

func summaryCutoff(messages []chat.Message) int {
	target := max(len(messages)-keepRecent, 1)
	for index := target; index > 0; index-- {
		if messages[index].Role == chat.RoleUser {
			return index
		}
	}
	for index := target + 1; index < len(messages); index++ {
		if messages[index].Role == chat.RoleUser {
			return index
		}
	}
	return 0
}

func (service *Service) Close() {
	if service == nil {
		return
	}
	service.once.Do(func() {
		service.mu.Lock()
		service.closed = true
		service.cancel()
		service.mu.Unlock()
		service.tasks.Wait()
	})
}
