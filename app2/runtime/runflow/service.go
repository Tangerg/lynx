// Package runflow owns Run admission, durable transcript/event commits and
// execution lifecycle. Agent Framework is consumed only through Executor.
package runflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/streamhub"
	"github.com/Tangerg/lynx/app2/runtime/transcriptflow"
)

type Store interface {
	DelegationStore
	GetSession(context.Context, session.ID) (session.Session, error)
	GetRun(context.Context, string) (rundomain.Record, error)
	GetOpenRootRun(context.Context, string) (rundomain.Record, error)
	ListOpenRunTree(context.Context, string) ([]rundomain.Record, error)
	ListRunningRuns(context.Context) ([]rundomain.Record, error)
	ListRuns(context.Context, string, []rundomain.Status, bool, int, *rundomain.Cursor) (rundomain.Page, error)
	CreateRun(context.Context, rundomain.Record, *transcript.Record, *conversationdomain.Record, []rundomain.EventRecord) error
	CreateScheduledRun(context.Context, ScheduledRunWrite) (bool, error)
	CommitRun(context.Context, CommitWrite) error
	CommitTreeRecovery(context.Context, TreeRecoveryWrite) error
	CommitRunEvent(context.Context, RunEventWrite) error
	CommitRunItemEvents(context.Context, RunItemEventWrite) error
	ListItems(context.Context, string, string) ([]transcript.Record, error)
	PageItems(context.Context, transcript.Query) (transcript.Page, error)
	ListConversationMessages(context.Context, string) ([]conversationdomain.Record, error)
	ListRunEvents(context.Context, string, string, string) ([]rundomain.EventRecord, error)
	GetInterruptSet(context.Context, string) (protocol.PendingInterruptSet, error)
	CommitResume(context.Context, ResumeWrite) error
	CommitTreeResume(context.Context, TreeResumeWrite) error
	CommitWaitingTreeCancel(context.Context, WaitingTreeCancelWrite) error
	CommitWait(context.Context, WaitWrite) error
	CommitTreeWait(context.Context, TreeWaitWrite) error
	CommitSteer(context.Context, SteerWrite) error
	GetExecutorCheckpoint(context.Context, string) ([]byte, error)
}

type IDs interface{ New(string) (string, error) }

type Executor interface {
	Execute(context.Context, agentexec.Input) (agentexec.Output, error)
	Resume(context.Context, agentexec.ResumeInput) (agentexec.Output, error)
	CancelWaitingTree(context.Context, agentexec.WaitingTreeCancelInput) (agentexec.WaitingTreeCancelOutput, error)
}

type ModelSelection interface {
	DefaultSelection(context.Context) (string, string, error)
}

type Checkpoints interface {
	Snapshot(context.Context, string, string, string) error
}

type Publisher interface {
	Publish(protocol.RuntimeEvent)
}

type MemoryMaintenance interface {
	RunSettled()
}

type LifecycleHooks interface {
	Observe(context.Context, lifecyclehook.Invocation)
}

type ActiveRunError struct{ Run protocol.RunRef }

func (failure *ActiveRunError) Error() string {
	return fmt.Sprintf("%s: run %s is %s", protocol.ErrSessionHasActiveRun, failure.Run.ID, failure.Run.Status)
}

func (failure *ActiveRunError) Is(target error) bool { return target == protocol.ErrSessionHasActiveRun }

func (failure *ActiveRunError) Enrich(problem *protocol.ProblemData) {
	problem.ActiveRun = &protocol.ActiveRunRef{RunID: failure.Run.ID, Status: failure.Run.Status}
}

type Service struct {
	store     Store
	ids       IDs
	executor  Executor
	models    ModelSelection
	checkpoints Checkpoints
	hub       *streamhub.Hub
	events    Publisher
	memory    MemoryMaintenance
	hooks     LifecycleHooks
	now       func() time.Time
	lifetime  context.Context
	cancel    context.CancelFunc

	mu      sync.Mutex
	active  map[string]activeExecution
	locks   map[string]*sync.Mutex
	tasks   sync.WaitGroup
	closing bool
}

type activeExecution struct {
	cancel context.CancelFunc
	steers chan agentexec.Steer
	cancels chan agentexec.Cancel
}

type Config struct {
	Store Store
	IDs IDs
	Executor Executor
	Models ModelSelection
	Checkpoints Checkpoints
	Hub *streamhub.Hub
	Events Publisher
	Memory MemoryMaintenance
	Hooks LifecycleHooks
	Lifetime context.Context
	Clock func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.IDs == nil || config.Executor == nil || config.Models == nil || config.Hub == nil || config.Events == nil || config.Checkpoints == nil || config.Memory == nil || config.Hooks == nil {
		return nil, errors.New("runflow: stores, execution ports, events, memory, and lifecycle hooks are required")
	}
	if config.Lifetime == nil {
		return nil, errors.New("runflow: lifetime is required")
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	lifetime, cancel := context.WithCancel(config.Lifetime)
	return &Service{
		store: config.Store, ids: config.IDs, executor: config.Executor, models: config.Models, checkpoints: config.Checkpoints,
		hub: config.Hub, events: config.Events, memory: config.Memory, hooks: config.Hooks,
		now: clock, lifetime: lifetime, cancel: cancel,
		active: make(map[string]activeExecution), locks: make(map[string]*sync.Mutex),
	}, nil
}

type StartCommand struct {
	Request protocol.StartRunRequest
	Meta    protocol.RequestMeta
}

// ScheduledStart is the private atomic admission command for one manual or
// durable cron occurrence. Session and Run identities are allocated before
// dispatch so retries converge on one transaction rather than creating
// duplicate headless work.
type ScheduledStart struct {
	ScheduleID           string
	OccurrenceID         string
	SessionID            string
	RunID                string
	Title                string
	Workspace            string
	Selection            modelselection.Selection
	Instruction          string
	FiredAt              time.Time
	AllowMissingSchedule bool
}

type ScheduledRun struct {
	SessionID string
	RunID     string
}

// AutonomousStart is the private application command used by Goal orchestration.
// It deliberately is not a Lyra wire request: autonomous instructions belong to
// the exact conversation journal without pretending to be user-authored Items.
type AutonomousStart struct {
	SessionID    string
	Instruction  string
	Provider     string
	Model        string
	MaxSteps     int
	MaxBudgetUSD float64
	Claim        func(context.Context, string) error
}

type AutonomousRun struct {
	RunID, SegmentID string
	Events           iter.Seq[protocol.RunEvent]
}

type rootStart struct {
	request      protocol.StartRunRequest
	profile      protocol.RunProtocolProfile
	visibleInput bool
	claim        func(context.Context, string) error
	scheduled    *scheduledAdmission
	startedAt    time.Time
}

type scheduledAdmission struct {
	session              session.Session
	runID                string
	scheduleID           string
	occurrenceID         string
	firedAt              time.Time
	allowMissingSchedule bool
}

type openedRoot struct {
	runID, segmentID, userItemID string
}

// treeStream is the delivery/replay scope of one active root Segment. Event
// source identity remains on RunEvent; this value only says which root stream
// carries it.
type treeStream struct {
	rootRunID     string
	rootSegmentID string
}

func newTreeStream(rootRunID, rootSegmentID string) (treeStream, error) {
	if rootRunID == "" || rootSegmentID == "" {
		return treeStream{}, errors.New("runflow: root tree stream identity is incomplete")
	}
	return treeStream{rootRunID: rootRunID, rootSegmentID: rootSegmentID}, nil
}

func (service *Service) activeTreeStream(
	ctx context.Context,
	value rundomain.Run,
	segmentID string,
) (treeStream, error) {
	if value.ParentRunID() == "" {
		return newTreeStream(value.ID(), segmentID)
	}
	root, err := service.store.GetRun(ctx, value.RootRunID())
	if err != nil {
		return treeStream{}, err
	}
	if root.Run.ParentRunID() != "" || root.Run.Status() != rundomain.Running ||
		root.Run.ActiveSegmentID() == "" || root.Run.SessionID() != value.SessionID() {
		return treeStream{}, errors.New("runflow: delegated Run has no active root tree stream")
	}
	return newTreeStream(root.Run.ID(), root.Run.ActiveSegmentID())
}

// CommitWrite is the single terminal/update transaction for an admitted Run.
// ExpectedStatus and ExpectedSegmentID are the generation token: every caller
// must prove which durable lifecycle version it is replacing.
type CommitWrite struct {
	Run               rundomain.Record
	ExpectedStatus    rundomain.Status
	ExpectedSegmentID string
	Items             []transcript.Record
	Messages          []conversationdomain.Record
	ToolResults       []toolresult.Record
	Events            []rundomain.EventRecord
}

type ScheduledRunWrite struct {
	Session              session.Session
	Run                  rundomain.Record
	Opening              transcript.Record
	OpeningMessage       conversationdomain.Record
	Events               []rundomain.EventRecord
	ScheduleID           string
	OccurrenceID         string
	FiredAt              time.Time
	AcceptedAt           time.Time
	AllowMissingSchedule bool
}

func (service *Service) Start(ctx context.Context, command StartCommand) (
	*protocol.StartRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	opened, events, err := service.startRoot(ctx, rootStart{
		request: command.Request, profile: profile(command.Meta.ClientCapabilities), visibleInput: true,
	})
	if err != nil { return nil, nil, err }
	return &protocol.StartRunResponse{RunID: opened.runID, SegmentID: opened.segmentID, UserItemID: opened.userItemID}, events, nil
}

func (service *Service) StartScheduled(ctx context.Context, command ScheduledStart) (*ScheduledRun, error) {
	if command.ScheduleID == "" || command.SessionID == "" || command.RunID == "" ||
		strings.TrimSpace(command.Instruction) == "" || command.FiredAt.IsZero() {
		return nil, errors.New("runflow: scheduled admission is incomplete")
	}
	workspace, err := session.NewWorkspace(command.Workspace)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	createdSession, err := session.New(session.Create{
		ID: session.ID(command.SessionID), Title: command.Title,
		Workspace: workspace, Selection: command.Selection, Now: now,
	})
	if err != nil {
		return nil, err
	}
	opened, _, err := service.startRoot(ctx, rootStart{
		request: protocol.StartRunRequest{
			SessionID: command.SessionID,
			Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: command.Instruction}},
			Provider: command.Selection.Provider(), Model: command.Selection.Model(),
		},
		profile: protocol.RunProtocolProfile{
			RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: protocol.InterruptTypes(),
		},
		visibleInput: true, startedAt: now,
		scheduled: &scheduledAdmission{
			session: createdSession, runID: command.RunID, scheduleID: command.ScheduleID,
			occurrenceID: command.OccurrenceID, firedAt: command.FiredAt.UTC(),
			allowMissingSchedule: command.AllowMissingSchedule,
		},
	})
	if err != nil {
		return nil, err
	}
	return &ScheduledRun{SessionID: command.SessionID, RunID: opened.runID}, nil
}

func (service *Service) StartAutonomous(ctx context.Context, command AutonomousStart) (*AutonomousRun, error) {
	if strings.TrimSpace(command.Instruction) == "" || command.Claim == nil {
		return nil, errors.New("runflow: autonomous instruction and ownership claim are required")
	}
	opened, events, err := service.startRoot(ctx, rootStart{
		request: protocol.StartRunRequest{
			SessionID: command.SessionID,
			Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: command.Instruction}},
			Provider: command.Provider, Model: command.Model,
			MaxSteps: command.MaxSteps, MaxBudgetUSD: command.MaxBudgetUSD,
		},
		profile: protocol.RunProtocolProfile{
			RequiredFeatures: []protocol.RunProtocolFeature{},
			InterruptTypes: protocol.InterruptTypes(),
		},
		claim: command.Claim,
	})
	if err != nil { return nil, err }
	return &AutonomousRun{RunID: opened.runID, SegmentID: opened.segmentID, Events: events}, nil
}

func (service *Service) startRoot(ctx context.Context, command rootStart) (*openedRoot, iter.Seq[protocol.RunEvent], error) {
	request := command.request
	var storedSession session.Session
	var err error
	if command.scheduled != nil {
		storedSession = command.scheduled.session
		if storedSession.ID() != session.ID(request.SessionID) {
			return nil, nil, errors.New("runflow: scheduled Session identity changed")
		}
	} else {
		storedSession, err = service.store.GetSession(ctx, session.ID(request.SessionID))
		if err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return nil, nil, protocol.ErrSessionNotFound
			}
			return nil, nil, err
		}
		if existing, err := service.store.GetOpenRootRun(ctx, request.SessionID); err == nil {
			presented, presentErr := presentRecord(existing)
			if presentErr != nil {
				return nil, nil, presentErr
			}
			return nil, nil, &ActiveRunError{Run: *presented}
		} else if !errors.Is(err, rundomain.ErrNotFound) {
			return nil, nil, err
		}
	}
	providerID, model, err := service.selection(ctx, request, storedSession)
	if err != nil {
		return nil, nil, err
	}
	runID := ""
	if command.scheduled != nil {
		runID = command.scheduled.runID
	} else {
		runID, err = service.ids.New("run_")
		if err != nil {
			return nil, nil, err
		}
	}
	segmentID, err := service.ids.New("seg_")
	if err != nil {
		return nil, nil, err
	}
	itemID := ""
	if command.visibleInput {
		itemID, err = service.ids.New("itm_")
		if err != nil { return nil, nil, err }
	}
	now := command.startedAt.UTC()
	if now.IsZero() {
		now = service.now().UTC()
	}
	aggregate, err := rundomain.New(rundomain.Start{
		ID: runID, SessionID: request.SessionID, SegmentID: segmentID,
		Provider: providerID, Model: model, Now: now,
	})
	if err != nil {
		return nil, nil, err
	}
	facts := runFacts{
		Metrics: protocol.RunMetrics{}, Limits: runLimits(request),
		Profile: command.profile, EventOrdinal: 1,
	}
	if command.visibleInput { facts.EventOrdinal = 2 }
	record, err := makeRecord(aggregate, facts)
	if err != nil {
		return nil, nil, err
	}
	var userItem *protocol.Item
	var opening *transcript.Record
	if command.visibleInput {
		item := protocol.Item{
			ID: itemID, RunID: runID, Status: protocol.ItemStatusCompleted,
			CreatedAt: now, Type: protocol.ItemTypeUserMessage, Content: request.Input,
		}
		stored, err := itemRecord(request.SessionID, item, 0)
		if err != nil { return nil, nil, err }
		userItem, opening = &item, &stored
	}
	presented, err := presentRecord(record)
	if err != nil {
		return nil, nil, err
	}
	stream, err := newTreeStream(runID, segmentID)
	if err != nil {
		return nil, nil, err
	}
	events, persisted, err := service.startEvents(runID, segmentID, stream, *presented, userItem, now)
	if err != nil {
		return nil, nil, err
	}
	conversation := []agentexec.Message{}
	nextMessageOrdinal := 0
	if command.scheduled == nil {
		conversation, nextMessageOrdinal, err = service.conversation(ctx, request.SessionID)
		if err != nil {
			return nil, nil, err
		}
	}
	userMessage, err := agentexec.UserMessage(request.Input)
	if err != nil { return nil, nil, err }
	sessionStart := len(conversation) == 0
	conversation = append(conversation, userMessage)
	messageBody, err := json.Marshal(userMessage)
	if err != nil { return nil, nil, err }
	openingMessage := conversationdomain.Record{SessionID: request.SessionID, RunID: runID, Ordinal: nextMessageOrdinal, Body: messageBody}
	created := true
	if command.scheduled != nil {
		created, err = service.store.CreateScheduledRun(ctx, ScheduledRunWrite{
			Session: storedSession, Run: record, Opening: *opening, OpeningMessage: openingMessage,
			Events: persisted, ScheduleID: command.scheduled.scheduleID,
			OccurrenceID: command.scheduled.occurrenceID, FiredAt: command.scheduled.firedAt,
			AcceptedAt: now, AllowMissingSchedule: command.scheduled.allowMissingSchedule,
		})
	} else {
		err = service.store.CreateRun(ctx, record, opening, &openingMessage, persisted)
	}
	if err != nil {
		if existing, lookupErr := service.store.GetOpenRootRun(ctx, request.SessionID); lookupErr == nil {
			active, presentErr := presentRecord(existing)
			if presentErr == nil {
				return nil, nil, &ActiveRunError{Run: *active}
			}
		}
		return nil, nil, err
	}
	if !created {
		existing, err := service.store.GetRun(ctx, runID)
		if err != nil {
			return nil, nil, err
		}
		return &openedRoot{runID: runID, segmentID: existing.Run.ActiveSegmentID()}, nil, nil
	}
	service.publishLifecycleChange(ctx, record.Run)
	if command.claim != nil {
		if err := command.claim(ctx, runID); err != nil {
			service.settleUnlaunched(runID)
			return nil, nil, fmt.Errorf("runflow: claim autonomous run %s: %w", runID, err)
		}
		if err := ctx.Err(); err != nil {
			service.settleUnlaunched(runID)
			return nil, nil, fmt.Errorf("runflow: autonomous run canceled before launch: %w", err)
		}
	}
	stream := service.hub.SubscribeRun(ctx, runID, segmentID, events)
	if !service.launchExecution(
		runID,
		segmentID,
		storedSession.Workspace().Path(),
		conversation,
		sessionStart,
		command.visibleInput,
	) {
		service.settleUnlaunched(runID)
	}
	return &openedRoot{runID: runID, segmentID: segmentID, userItemID: itemID}, stream, nil
}

func (service *Service) launchExecution(
	runID string,
	segmentID string,
	workspace string,
	conversation []agentexec.Message,
	sessionStart bool,
	userPromptSubmit bool,
) bool {
	service.mu.Lock()
	if service.closing {
		service.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(service.lifetime)
	steers := make(chan agentexec.Steer, 32)
	cancels := make(chan agentexec.Cancel, 8)
	service.active[runID] = activeExecution{cancel: cancel, steers: steers, cancels: cancels}
	service.tasks.Add(1)
	service.mu.Unlock()
	go func() {
		defer service.tasks.Done()
		defer func() {
			service.mu.Lock()
			delete(service.active, runID)
			service.mu.Unlock()
			cancel()
		}()
		record, err := service.store.GetRun(ctx, runID)
		if err != nil || record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() != segmentID {
			return
		}
		live := newLiveProjector(service, record, segmentID)
		output, executeErr := service.executor.Execute(ctx, agentexec.Input{
			Provider: record.Run.Provider(), Model: record.Run.Model(), Workspace: workspace,
			SessionID: record.Run.SessionID(), RunID: runID, SegmentID: segmentID,
			IsRootRun: record.Run.ParentRunID() == "", SessionStart: sessionStart,
			UserPromptSubmit: userPromptSubmit,
			Subagents: runUsesSubagents(record.Body), Delegation: service,
			Conversation: conversation, Steers: steers, Cancels: cancels,
			MaxSteps: runMaxSteps(record.Body), Live: live,
		})
		live.Close()
		service.finishExecution(runID, segmentID, workspace, output, executeErr)
	}()
	return true
}

func runMaxSteps(body []byte) int {
	facts, err := decodeFacts(body)
	if err != nil || facts.Limits == nil {
		return 0
	}
	return facts.Limits.MaxSteps
}

func runUsesSubagents(body []byte) bool {
	facts, err := decodeFacts(body)
	if err != nil {
		return false
	}
	for _, feature := range facts.Profile.RequiredFeatures {
		if feature == protocol.RunProtocolFeatureSubagents {
			return true
		}
	}
	return false
}

func (service *Service) conversation(ctx context.Context, sessionID string) ([]agentexec.Message, int, error) {
	records, err := service.store.ListConversationMessages(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	messages := make([]agentexec.Message, 0, len(records))
	nextOrdinal := 0
	for _, record := range records {
		var message agentexec.Message
		if err := json.Unmarshal(record.Body, &message); err != nil { return nil, 0, fmt.Errorf("runflow: decode conversation message: %w", err) }
		if err := message.Validate(); err != nil { return nil, 0, err }
		messages = append(messages, message)
		if record.Ordinal >= nextOrdinal { nextOrdinal = record.Ordinal + 1 }
	}
	return messages, nextOrdinal, nil
}

func (service *Service) finishExecution(runID, segmentID, workspace string, output agentexec.Output, executeErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if childErr := service.finishDelegatedExecutions(ctx, output.Children); childErr != nil && executeErr == nil {
		executeErr = childErr
	}
	lock := service.runLock(runID)
	lock.Lock()
	defer lock.Unlock()
	record, err := service.store.GetRun(ctx, runID)
	if err != nil || record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() != segmentID {
		return
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return
	}
	now := service.now().UTC()
	if executeErr == nil && output.Waiting != nil && output.Waiting.Tree {
		if err := service.parkTreeExecution(ctx, record, segmentID, *output.Waiting, now); err == nil {
			return
		} else {
			executeErr = err
		}
	}
	mergeRunUsage(&facts.Metrics, output.Usage, output.ModelCalls)
	if output.ContextTokens > 0 {
		facts.ContextTokens = output.ContextTokens
	}
	executionStatus := output.Status
	if executionStatus == "" {
		executionStatus = agentexec.ExecutionCompleted
	}
	terminalProjection := executeErr == nil && output.Waiting == nil && executionStatus == agentexec.ExecutionCompleted
	projectedFacts := facts
	projection, projectionErr := service.projectExecution(
		ctx,
		record,
		segmentID,
		output,
		&projectedFacts,
		executionProjectionPolicy{terminal: terminalProjection, sessionConversation: true},
	)
	if projectionErr != nil && executeErr == nil {
		executeErr = projectionErr
	} else if projectionErr == nil {
		facts = projectedFacts
	}
	items := projection.items
	events := projection.events
	if executeErr == nil && output.Waiting != nil {
		if err := service.parkExecution(ctx, record, facts, segmentID, *output.Waiting, output.Tools, projection, now); err == nil {
			return
		} else {
			executeErr = err
		}
	}
	if checkpointErr := service.checkpoints.Snapshot(ctx, record.Run.SessionID(), workspace, runID); checkpointErr != nil && executeErr == nil {
		executeErr = fmt.Errorf("checkpoint run boundary: %w", checkpointErr)
	}
	outcome := rundomain.Completed
	detail := ""
	problem := (*protocol.ProblemData)(nil)
	if executeErr != nil {
		if errors.Is(executeErr, agentexec.ErrAdmissionRejected) {
			outcome = rundomain.Failed
			detail = strings.TrimSpace(strings.TrimPrefix(
				executeErr.Error(),
				agentexec.ErrAdmissionRejected.Error()+":",
			))
			if detail == "" {
				detail = "blocked by lifecycle policy"
			}
			detail = boundedHookText(detail, lifecyclehook.MaxReasonBytes)
			problem = &protocol.ProblemData{Type: protocol.ProblemInvalidRequest, Detail: detail}
		} else if errors.Is(executeErr, context.Canceled) && service.lifetime.Err() != nil {
			outcome = rundomain.Lost
			detail = lostRunDetail
			problem = &protocol.ProblemData{Type: protocol.ProblemRunLost, Detail: detail}
		} else {
			outcome = rundomain.Failed
			detail = "the agent execution failed"
			problem = &protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: detail}
		}
	} else {
		detail = output.Detail
		switch executionStatus {
		case agentexec.ExecutionCompleted:
			outcome = rundomain.Completed
		case agentexec.ExecutionCanceled:
			if service.lifetime.Err() != nil {
				outcome = rundomain.Lost
				detail = lostRunDetail
				problem = &protocol.ProblemData{Type: protocol.ProblemRunLost, Detail: detail}
			} else {
				outcome = rundomain.Canceled
			}
		case agentexec.ExecutionTimedOut:
			outcome = rundomain.TimedOut
		case agentexec.ExecutionFailed, agentexec.ExecutionKilled:
			outcome = rundomain.Failed
			if detail == "" {
				detail = "the agent execution failed"
			}
			problem = &protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: detail}
		default:
			outcome = rundomain.Failed
			detail = "the agent execution returned an unknown terminal status"
			problem = &protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: detail}
		}
	}
	if err := record.Run.Finish(segmentID, outcome, detail, now); err != nil {
		return
	}
	finished, err := service.event(runID, segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamSegmentFinished,
		Outcome: segmentOutcome(outcome, problem, detail),
		Metrics: &facts.Metrics,
	}, now)
	if err != nil {
		return
	}
	events = append(events, finished)
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return
	}
	firstOrdinal := facts.EventOrdinal - len(events) + 1
	stream, err := newTreeStream(runID, segmentID)
	if err != nil {
		return
	}
	persisted, err := persistEvents(events, firstOrdinal, stream)
	if err != nil {
		return
	}
	if err := service.store.CommitRun(ctx, CommitWrite{Run: record, ExpectedStatus: rundomain.Running, ExpectedSegmentID: segmentID, Items: items, Messages: projection.messages, ToolResults: projection.results, Events: persisted}); err != nil {
		return
	}
	service.publishLifecycleChange(ctx, record.Run)
	for _, event := range events {
		service.hub.PublishRun(runID, segmentID, event)
	}
	if record.Run.ParentRunID() == "" {
		service.memory.RunSettled()
	}
}

func (service *Service) Get(ctx context.Context, runID string) (*protocol.RunRef, error) {
	record, err := service.store.GetRun(ctx, runID)
	if err != nil {
		return nil, projectRunLookup(err)
	}
	return presentRecord(record)
}

func (service *Service) List(ctx context.Context, request protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	cursor, err := decodeRunCursor(request.Cursor, request)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", protocol.ErrInvalidParams)
	}
	statuses := make([]rundomain.Status, len(request.Statuses))
	for index, status := range request.Statuses {
		statuses[index] = rundomain.Status(status)
	}
	page, err := service.store.ListRuns(ctx, request.SessionID, statuses, request.IncludeDescendants, request.Limit, cursor)
	if err != nil {
		return nil, err
	}
	data := make([]protocol.RunRef, 0, len(page.Records))
	for _, record := range page.Records {
		presented, err := presentRecord(record)
		if err != nil {
			return nil, err
		}
		data = append(data, *presented)
	}
	next := ""
	if page.Next != nil {
		next = encodeRunCursor(*page.Next, request)
	}
	return protocol.NewPageWithCursor(data, next), nil
}

func (service *Service) Subscribe(ctx context.Context, request protocol.SubscribeRunRequest, afterEventID string) (
	*protocol.SubscribeRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	record, err := service.store.GetRun(ctx, request.RunID)
	if err != nil {
		return nil, nil, projectRunLookup(err)
	}
	if record.Run.ParentRunID() != "" {
		return nil, nil, protocol.ErrRunNotRoot
	}
	if record.Run.Status() == rundomain.Waiting {
		return nil, nil, protocol.ErrRunWaiting
	}
	if record.Run.Status() == rundomain.Finished {
		return nil, nil, protocol.ErrRunFinished
	}
	if record.Run.ActiveSegmentID() != request.SegmentID {
		return nil, nil, protocol.ErrStaleSegment
	}
	subscriptionCtx, cancelSubscription := context.WithCancel(ctx)
	live := service.hub.SubscribeRun(
		subscriptionCtx,
		request.RunID,
		request.SegmentID,
		nil,
	)
	replay, head, err := service.replay(ctx, request.RunID, request.SegmentID, afterEventID)
	if err != nil {
		cancelSubscription()
		return nil, nil, err
	}
	return &protocol.SubscribeRunResponse{
		RunID: request.RunID, SegmentID: request.SegmentID, HeadEventID: head,
	}, mergeRunEvents(cancelSubscription, request.RunID, request.SegmentID, replay, live), nil
}

type CancelCommand struct {
	Request protocol.CancelRunRequest
	Meta    protocol.RequestMeta
}

func (service *Service) Cancel(
	ctx context.Context,
	request protocol.CancelRunRequest,
) (*protocol.CancelRunResponse, error) {
	return service.CancelWith(ctx, CancelCommand{Request: request})
}

func (service *Service) CancelWith(
	ctx context.Context,
	command CancelCommand,
) (*protocol.CancelRunResponse, error) {
	request := command.Request
	record, err := service.store.GetRun(ctx, request.RunID)
	if err != nil {
		return nil, projectRunLookup(err)
	}
	if record.Run.ParentRunID() != "" && !clientNegotiatedSubagents(command.Meta.ClientCapabilities) {
		return nil, protocol.ErrCapabilityNotNeg
	}
	if record.Run.Status() == rundomain.Finished {
		return nil, protocol.ErrRunFinished
	}
	if record.Run.Status() == rundomain.Waiting {
		if record.Run.ParentRunID() != "" {
			return service.cancelWaitingTreeChild(ctx, record, request.Reason)
		}
		if runUsesSubagents(record.Body) {
			return service.cancelWaitingTreeRoot(ctx, record, request.Reason)
		}
		return service.cancelSingleWaiting(ctx, record, request.Reason)
	}
	return service.cancelLiveTreeMember(ctx, record, request.Reason)
}

func (service *Service) cancelLiveTreeMember(
	ctx context.Context,
	target rundomain.Record,
	reason string,
) (*protocol.CancelRunResponse, error) {
	rootRunID := target.Run.ID()
	if target.Run.ParentRunID() != "" {
		rootRunID = target.Run.RootRunID()
	}
	service.mu.Lock()
	active, found := service.active[rootRunID]
	service.mu.Unlock()
	if !found {
		return nil, protocol.ErrSessionBusy
	}
	if strings.TrimSpace(reason) == "" {
		reason = "canceled by user"
	}
	result := make(chan agentexec.CancelResult, 1)
	control := agentexec.Cancel{RunID: target.Run.ID(), Reason: reason, Result: result}
	select {
	case active.cancels <- control:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-service.lifetime.Done():
		return nil, protocol.ErrSessionBusy
	}
	var accepted agentexec.CancelResult
	select {
	case accepted = <-result:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-service.lifetime.Done():
		return nil, protocol.ErrSessionBusy
	}
	if accepted.Err != nil {
		return nil, accepted.Err
	}
	if err := service.finishDelegatedExecutions(context.WithoutCancel(ctx), accepted.Children); err != nil {
		return nil, err
	}
	settled, err := service.waitRunFinished(ctx, target.Run.ID())
	if err != nil {
		return nil, err
	}
	presented, err := presentRecord(settled)
	if err != nil {
		return nil, err
	}
	if target.Run.ParentRunID() == "" {
		return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: *presented}, nil
	}
	root, err := service.store.GetRun(ctx, rootRunID)
	if err != nil {
		return nil, err
	}
	presentedRoot, err := presentRecord(root)
	if err != nil {
		return nil, err
	}
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunChild, Run: *presented, RootRun: presentedRoot,
	}, nil
}

func (service *Service) waitRunFinished(ctx context.Context, runID string) (rundomain.Record, error) {
	const interval = 20 * time.Millisecond
	for {
		record, err := service.store.GetRun(ctx, runID)
		if err != nil {
			return rundomain.Record{}, err
		}
		if record.Run.Status() == rundomain.Finished {
			return record, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return rundomain.Record{}, ctx.Err()
		case <-service.lifetime.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return rundomain.Record{}, protocol.ErrSessionBusy
		}
	}
}

func (service *Service) cancelSingleWaiting(
	ctx context.Context,
	record rundomain.Record,
	reason string,
) (*protocol.CancelRunResponse, error) {
	lock := service.runLock(record.Run.ID())
	lock.Lock()
	defer lock.Unlock()
	current, err := service.store.GetRun(ctx, record.Run.ID())
	if err != nil {
		return nil, err
	}
	if current.Run.Status() != rundomain.Waiting {
		return nil, rundomain.ErrInvalidTransition
	}
	if strings.TrimSpace(reason) == "" {
		reason = "canceled by user"
	}
	segmentID, err := service.ids.New("seg_")
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	if err := current.Run.Resume(segmentID, now); err != nil {
		return nil, err
	}
	facts, err := decodeFacts(current.Body)
	if err != nil {
		return nil, err
	}
	current, err = makeRecord(current.Run, facts)
	if err != nil {
		return nil, err
	}
	presentedRunning, err := presentRecord(current)
	if err != nil {
		return nil, err
	}
	started, err := service.event(current.Run.ID(), segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamSegmentStarted, Run: presentedRunning,
	}, now)
	if err != nil {
		return nil, err
	}
	if _, err := service.store.GetInterruptSet(ctx, current.Run.ID()); err != nil {
		return nil, protocol.ErrInterruptNotOpen
	}
	if _, err := service.store.GetExecutorCheckpoint(ctx, current.Run.ID()); err != nil {
		return nil, protocol.ErrInterruptNotOpen
	}
	storedItems, err := service.store.ListItems(ctx, "", current.Run.ID())
	if err != nil {
		return nil, err
	}
	updatedItems := make([]transcript.Record, 0)
	itemsByID := make(map[string]transcript.Record, len(storedItems))
	events := []protocol.RunEvent{started}
	for _, stored := range storedItems {
		var item protocol.Item
		if err := json.Unmarshal(stored.Body, &item); err != nil {
			return nil, err
		}
		if item.Status == protocol.ItemStatusRunning {
			if err := interruptRunningItem(&item, protocol.ProblemToolCanceled, reason, now); err != nil {
				return nil, err
			}
			stored.Body, err = json.Marshal(item)
			if err != nil {
				return nil, err
			}
			updatedItems = append(updatedItems, stored)
			completed, err := service.event(current.Run.ID(), segmentID, &facts, protocol.StreamEvent{
				Type: protocol.StreamItemCompleted, Item: &item,
			}, now)
			if err != nil {
				return nil, err
			}
			events = append(events, completed)
		}
		itemsByID[stored.ID] = stored
	}
	conversation, err := service.store.ListConversationMessages(ctx, current.Run.SessionID())
	if err != nil {
		return nil, err
	}
	conversationResults, err := projectConversation(
		current, agentexec.Output{}, itemsByID, conversation,
	)
	if err != nil {
		return nil, err
	}
	storedSession, err := service.store.GetSession(ctx, session.ID(current.Run.SessionID()))
	if err != nil {
		return nil, err
	}
	if err := service.checkpoints.Snapshot(ctx, current.Run.SessionID(), storedSession.Workspace().Path(), current.Run.ID()); err != nil {
		return nil, fmt.Errorf("runflow: checkpoint canceled waiting Run: %w", err)
	}
	if err := current.Run.Finish(segmentID, rundomain.Canceled, reason, now); err != nil {
		return nil, err
	}
	finished, err := service.event(current.Run.ID(), segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamSegmentFinished,
		Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentCanceled, Detail: reason},
		Metrics: &facts.Metrics,
	}, now)
	if err != nil {
		return nil, err
	}
	events = append(events, finished)
	current, err = makeRecord(current.Run, facts)
	if err != nil {
		return nil, err
	}
	stream, err := newTreeStream(current.Run.ID(), segmentID)
	if err != nil {
		return nil, err
	}
	persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1, stream)
	if err != nil {
		return nil, err
	}
	if err := service.store.CommitRun(ctx, CommitWrite{
		Run: current, ExpectedStatus: rundomain.Waiting,
		Items: updatedItems, Messages: conversationResults, Events: persisted,
	}); err != nil {
		return nil, err
	}
	service.publishLifecycleChange(ctx, current.Run)
	service.publishInterruptChange(current.Run)
	for _, event := range events {
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
	}
	presented, err := presentRecord(current)
	if err != nil {
		return nil, err
	}
	return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: *presented}, nil
}

func clientNegotiatedSubagents(capabilities *protocol.ClientCapabilities) bool {
	return capabilities != nil && capabilities.Features[protocol.FeatureSubagents]
}

func (service *Service) Steer(ctx context.Context, request protocol.SteerRunRequest) error {
	if len(request.Input) == 0 {
		return fmt.Errorf("%w: steer input is required", protocol.ErrInvalidParams)
	}
	lock := service.runLock(request.RunID)
	lock.Lock()
	defer lock.Unlock()
	record, err := service.store.GetRun(ctx, request.RunID)
	if err != nil {
		return projectRunLookup(err)
	}
	if record.Run.ParentRunID() != "" {
		return protocol.ErrRunNotRoot
	}
	switch record.Run.Status() {
	case rundomain.Waiting:
		return protocol.ErrRunWaiting
	case rundomain.Finished:
		return protocol.ErrRunFinished
	}
	if record.Run.ActiveSegmentID() != request.ExpectedSegmentID {
		return protocol.ErrStaleSegment
	}
	service.mu.Lock()
	active, ok := service.active[request.RunID]
	service.mu.Unlock()
	if !ok {
		return protocol.ErrRunFinished
	}
	result := make(chan error, 1)
	command := agentexec.Steer{Input: slices.Clone(request.Input), Result: result}
	select {
	case active.steers <- command:
	case <-ctx.Done():
		return ctx.Err()
	case <-service.lifetime.Done():
		return protocol.ErrRunFinished
	}
	select {
	case err := <-result:
		if err != nil {
			return err
		}
		itemID, err := service.ids.New("itm_")
		if err != nil { active.cancel(); return err }
		now := service.now().UTC()
		item := protocol.Item{ID: itemID, RunID: request.RunID, Status: protocol.ItemStatusCompleted, CreatedAt: now, Type: protocol.ItemTypeUserMessage, Content: slices.Clone(request.Input)}
		items, err := service.store.ListItems(ctx, "", request.RunID)
		if err != nil { active.cancel(); return err }
		storedItem, err := itemRecord(record.Run.SessionID(), item, nextOrdinal(items, request.RunID))
		if err != nil { active.cancel(); return err }
		message, err := agentexec.UserMessage(request.Input)
		if err != nil { active.cancel(); return err }
		body, err := json.Marshal(message)
		if err != nil { active.cancel(); return err }
		messages, err := service.store.ListConversationMessages(ctx, record.Run.SessionID())
		if err != nil { active.cancel(); return err }
		storedMessage := conversationdomain.Record{SessionID: record.Run.SessionID(), RunID: request.RunID, Ordinal: nextConversationOrdinal(messages), Body: body}
		facts, err := decodeFacts(record.Body)
		if err != nil { active.cancel(); return err }
		if err := record.Run.Touch(request.ExpectedSegmentID, now); err != nil { active.cancel(); return err }
		event, err := service.event(request.RunID, request.ExpectedSegmentID, &facts, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}, now)
		if err != nil { active.cancel(); return err }
		record, err = makeRecord(record.Run, facts)
		if err != nil { active.cancel(); return err }
		stream, err := newTreeStream(request.RunID, request.ExpectedSegmentID)
		if err != nil { active.cancel(); return err }
		persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal, stream)
		if err != nil { active.cancel(); return err }
		if err := service.store.CommitSteer(ctx, SteerWrite{Run: record, ExpectedSegmentID: request.ExpectedSegmentID, Item: storedItem, Message: storedMessage, Event: persisted[0]}); err != nil { active.cancel(); return err }
		service.publishRunChange(record.Run)
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-service.lifetime.Done():
		return protocol.ErrRunFinished
	}
}

func mergeRunEvents(
	cancel context.CancelFunc,
	rootRunID string,
	rootSegmentID string,
	replay []protocol.RunEvent,
	live iter.Seq[protocol.RunEvent],
) iter.Seq[protocol.RunEvent] {
	return func(yield func(protocol.RunEvent) bool) {
		defer cancel()
		seen := make(map[string]struct{}, len(replay))
		for _, event := range replay {
			seen[event.EventID] = struct{}{}
			if !yield(event) || closesTreeStream(rootRunID, rootSegmentID, event) {
				return
			}
		}
		for event := range live {
			if _, duplicate := seen[event.EventID]; duplicate {
				continue
			}
			seen[event.EventID] = struct{}{}
			if !yield(event) || closesTreeStream(rootRunID, rootSegmentID, event) {
				return
			}
		}
	}
}

func closesTreeStream(rootRunID, rootSegmentID string, event protocol.RunEvent) bool {
	return event.RunID == rootRunID && event.SegmentID == rootSegmentID &&
		event.Event.Type == protocol.StreamSegmentFinished
}

func (service *Service) publishLifecycleChange(ctx context.Context, value rundomain.Run) {
	service.publishLifecycleChangeWithResult(ctx, value, "")
}

func (service *Service) publishLifecycleChangeWithResult(
	ctx context.Context,
	value rundomain.Run,
	terminalResult string,
) {
	service.publishRunChange(value)
	service.events.Publish(protocol.RuntimeEvent{
		Type:       protocol.RuntimeSessionsChanged,
		SessionIDs: []string{value.SessionID()},
	})
	if value.Status() == rundomain.Finished {
		service.observeTerminalHooks(ctx, value, terminalResult)
	}
}

func (service *Service) observeTerminalHooks(
	ctx context.Context,
	value rundomain.Run,
	terminalResult string,
) {
	storedSession, err := service.store.GetSession(ctx, session.ID(value.SessionID()))
	if err != nil {
		return
	}
	reason := string(value.Outcome())
	if value.Detail() != "" {
		reason += ": " + value.Detail()
	}
	reason = boundedHookText(reason, lifecyclehook.MaxReasonBytes)
	if value.ParentRunID() != "" {
		result, resultTruncated := boundedHookMaterial(
			terminalResult,
			lifecyclehook.MaxResultBytes,
		)
		errorText := ""
		if value.Outcome() != rundomain.Completed {
			errorText = boundedHookText(value.Detail(), lifecyclehook.MaxReasonBytes)
		}
		service.hooks.Observe(ctx, lifecyclehook.Invocation{
			Event: lifecyclehook.SubagentStop,
			SessionID: value.SessionID(), RunID: value.ID(),
			Workspace: storedSession.Workspace().Path(),
			Subagent: &lifecyclehook.SubagentInput{
				RunID: value.ID(), ParentRunID: value.ParentRunID(),
				Status: hookSubagentStatus(value.Outcome()),
				Result: result, Error: errorText,
				ResultTruncated: resultTruncated,
			},
		})
	}
	service.hooks.Observe(ctx, lifecyclehook.Invocation{
		Event: lifecyclehook.Stop,
		SessionID: value.SessionID(), RunID: value.ID(),
		Workspace: storedSession.Workspace().Path(), Reason: reason,
	})
}

func (service *Service) observeWaitingHook(ctx context.Context, value rundomain.Run) {
	storedSession, err := service.store.GetSession(ctx, session.ID(value.SessionID()))
	if err != nil {
		return
	}
	service.hooks.Observe(ctx, lifecyclehook.Invocation{
		Event: lifecyclehook.Notification,
		SessionID: value.SessionID(), RunID: value.ID(),
		Workspace: storedSession.Workspace().Path(), Reason: "waiting for user input",
	})
}

func hookSubagentStatus(outcome rundomain.Outcome) lifecyclehook.SubagentStatus {
	switch outcome {
	case rundomain.Completed:
		return lifecyclehook.SubagentCompleted
	case rundomain.TimedOut:
		return lifecyclehook.SubagentTimedOut
	case rundomain.MaxSteps:
		return lifecyclehook.SubagentMaxSteps
	case rundomain.MaxBudget:
		return lifecyclehook.SubagentMaxBudget
	case rundomain.Canceled:
		return lifecyclehook.SubagentCanceled
	case rundomain.Lost:
		return lifecyclehook.SubagentLost
	default:
		return lifecyclehook.SubagentFailed
	}
}

func boundedHookText(value string, limit int) string {
	result, _ := boundedHookMaterial(value, limit)
	return result
}

func boundedHookMaterial(value string, limit int) (string, bool) {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func (service *Service) publishRunChange(value rundomain.Run) {
	service.events.Publish(protocol.RuntimeEvent{
		Type:       protocol.RuntimeRunsChanged,
		SessionIDs: []string{value.SessionID()},
		RunIDs:     []string{value.ID()},
	})
}

func (service *Service) publishInterruptChange(value rundomain.Run) {
	service.events.Publish(protocol.RuntimeEvent{
		Type:       protocol.RuntimeInterruptsChanged,
		SessionIDs: []string{value.SessionID()},
		RunIDs:     []string{value.ID()},
	})
}

func (service *Service) Items(ctx context.Context, request protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	order := transcript.OrderAscending
	switch request.Order {
	case "", protocol.ItemOrderAsc:
	case protocol.ItemOrderDesc:
		order = transcript.OrderDescending
	default:
		return nil, fmt.Errorf("%w: item order is invalid", protocol.ErrInvalidParams)
	}
	query := transcript.Query{
		Order: order,
		Limit: request.Limit,
	}
	switch request.Scope.Type {
	case protocol.ItemScopeSession:
		if request.Scope.SessionID == "" || request.Scope.RunID != "" || request.Scope.IncludeDescendants {
			return nil, fmt.Errorf("%w: session item scope is invalid", protocol.ErrInvalidParams)
		}
		query.Scope.SessionID = request.Scope.SessionID
	case protocol.ItemScopeRun:
		if request.Scope.RunID == "" || request.Scope.SessionID != "" {
			return nil, fmt.Errorf("%w: run item scope is invalid", protocol.ErrInvalidParams)
		}
		query.Scope.RunID = request.Scope.RunID
		query.Scope.IncludeDescendants = request.Scope.IncludeDescendants
	default:
		return nil, fmt.Errorf("%w: item scope is invalid", protocol.ErrInvalidParams)
	}
	cursor, err := decodeItemCursor(request.Cursor, request)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", protocol.ErrInvalidParams)
	}
	query.Cursor = cursor
	page, err := service.store.PageItems(ctx, query)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, protocol.ErrSessionNotFound
		}
		return nil, projectRunLookup(err)
	}
	items := make([]protocol.Item, 0, len(page.Records))
	runIDs := make([]string, 0, len(page.Records))
	seenRunIDs := make(map[string]bool)
	for _, record := range page.Records {
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return nil, fmt.Errorf("runflow: decode item: %w", err)
		}
		items = append(items, item)
		if !seenRunIDs[item.RunID] {
			seenRunIDs[item.RunID] = true
			runIDs = append(runIDs, item.RunID)
		}
	}
	runs, err := service.runSummaryClosure(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	next := ""
	if page.Next != nil {
		next = encodeItemCursor(*page.Next, request)
	}
	return &protocol.ListItemsResponse{
		Page: *protocol.NewPageWithCursor(items, next),
		Runs: runs,
	}, nil
}

func (service *Service) runSummaryClosure(ctx context.Context, runIDs []string) ([]protocol.RunSummary, error) {
	result := make([]protocol.RunSummary, 0, len(runIDs))
	seen := make(map[string]bool)
	for _, runID := range runIDs {
		lineage := make([]protocol.RunSummary, 0, 2)
		for runID != "" && !seen[runID] {
			record, err := service.store.GetRun(ctx, runID)
			if err != nil {
				return nil, projectRunLookup(err)
			}
			presented, err := presentRecord(record)
			if err != nil {
				return nil, err
			}
			lineage = append(lineage, presented.RunSummary)
			runID = record.Run.ParentRunID()
		}
		slices.Reverse(lineage)
		for _, summary := range lineage {
			if seen[summary.ID] {
				continue
			}
			seen[summary.ID] = true
			result = append(result, summary)
		}
	}
	return result, nil
}

func (service *Service) replay(ctx context.Context, runID, segmentID, after string) ([]protocol.RunEvent, string, error) {
	records, err := service.store.ListRunEvents(ctx, runID, segmentID, after)
	if err != nil {
		if errors.Is(err, rundomain.ErrStaleSegment) {
			return nil, "", protocol.ErrReplayCursorInvalid
		}
		return nil, "", err
	}
	events := make([]protocol.RunEvent, 0, len(records))
	head := after
	for _, record := range records {
		if record.RootRunID != runID || record.RootSegmentID != segmentID {
			return nil, "", errors.New("runflow: replay store returned another tree stream")
		}
		var event protocol.RunEvent
		if err := json.Unmarshal(record.Body, &event); err != nil {
			return nil, "", fmt.Errorf("runflow: decode event: %w", err)
		}
		if event.RunID != record.RunID || event.SegmentID != record.SegmentID || event.EventID != record.EventID {
			return nil, "", errors.New("runflow: replay event identity differs from its journal record")
		}
		events = append(events, event)
		head = event.EventID
	}
	return events, head, nil
}

func (service *Service) startEvents(
	runID, segmentID string,
	stream treeStream,
	run protocol.RunRef,
	item *protocol.Item,
	now time.Time,
) ([]protocol.RunEvent, []rundomain.EventRecord, error) {
	firstID, err := service.ids.New("evt_")
	if err != nil {
		return nil, nil, err
	}
	events := []protocol.RunEvent{
		{RunID: runID, SegmentID: segmentID, EventID: firstID, Timestamp: now,
			Event: protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &run}},
	}
	if item != nil {
		secondID, err := service.ids.New("evt_")
		if err != nil { return nil, nil, err }
		events = append(events, protocol.RunEvent{
			RunID: runID, SegmentID: segmentID, EventID: secondID, Timestamp: now,
			Event: protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: item},
		})
	}
	persisted, err := persistEvents(events, 1, stream)
	return events, persisted, err
}

func (service *Service) event(
	runID, segmentID string,
	facts *runFacts,
	payload protocol.StreamEvent,
	now time.Time,
) (protocol.RunEvent, error) {
	id, err := service.ids.New("evt_")
	if err != nil {
		return protocol.RunEvent{}, err
	}
	facts.EventOrdinal++
	return protocol.RunEvent{
		RunID: runID, SegmentID: segmentID, EventID: id, Timestamp: now, Event: payload,
	}, nil
}

func persistEvents(
	events []protocol.RunEvent,
	firstOrdinal int,
	stream treeStream,
) ([]rundomain.EventRecord, error) {
	if _, err := newTreeStream(stream.rootRunID, stream.rootSegmentID); err != nil {
		return nil, err
	}
	records := make([]rundomain.EventRecord, 0, len(events))
	for index, event := range events {
		if event.RunID == "" || event.SegmentID == "" || event.EventID == "" {
			return nil, errors.New("runflow: persisted Run event identity is incomplete")
		}
		body, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		records = append(records, rundomain.EventRecord{
			RootRunID: stream.rootRunID, RootSegmentID: stream.rootSegmentID,
			RunID: event.RunID, SegmentID: event.SegmentID, EventID: event.EventID,
			Ordinal: firstOrdinal + index, Body: body, CreatedAt: event.Timestamp,
		})
	}
	return records, nil
}

type runFacts struct {
	Metrics protocol.RunMetrics `json:"metrics"`
	ContextTokens int64 `json:"contextTokens,omitempty"`
	Limits *protocol.RunLimits `json:"limits,omitempty"`
	Profile protocol.RunProtocolProfile `json:"profile"`
	EventOrdinal int `json:"eventOrdinal"`
}

func makeRecord(value rundomain.Run, facts runFacts) (rundomain.Record, error) {
	body, err := json.Marshal(facts)
	if err != nil {
		return rundomain.Record{}, fmt.Errorf("runflow: encode run facts: %w", err)
	}
	return rundomain.Record{Run: value, Body: body}, nil
}

func decodeFacts(body []byte) (runFacts, error) {
	var facts runFacts
	if err := json.Unmarshal(body, &facts); err != nil {
		return runFacts{}, fmt.Errorf("runflow: decode run facts: %w", err)
	}
	return facts, nil
}

func presentRecord(record rundomain.Record) (*protocol.RunRef, error) {
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return nil, err
	}
	value := record.Run
	summary := protocol.RunSummary{
		ID: value.ID(), SessionID: value.SessionID(), SpawnedByItemID: value.SpawnedByItemID(),
		ParentRunID: value.ParentRunID(), RootRunID: value.RootRunID(),
		Model: value.Model(), Provider: value.Provider(), Status: protocol.RunStatus(value.Status()),
		CreatedAt: value.CreatedAt(), FinishedAt: value.FinishedAt(),
	}
	if value.Status() == rundomain.Finished {
		summary.Outcome = &protocol.RunOutcome{Type: protocol.RunOutcomeType(value.Outcome())}
		if value.Outcome() == rundomain.Failed || value.Outcome() == rundomain.Lost {
			problemType := protocol.ProblemInternalError
			if value.Outcome() == rundomain.Lost { problemType = protocol.ProblemRunLost }
			summary.Outcome.Error = &protocol.ProblemData{Type: problemType, Detail: value.Detail()}
		} else {
			summary.Outcome.Detail = value.Detail()
		}
	}
	result := &protocol.RunRef{
		RunSummary: summary, ActiveSegmentID: value.ActiveSegmentID(), Metrics: facts.Metrics,
		ContextTokens: facts.ContextTokens, Limits: facts.Limits,
	}
	if value.ParentRunID() == "" {
		result.ProtocolProfile = facts.Profile
	}
	return result, nil
}

func segmentOutcome(outcome rundomain.Outcome, problem *protocol.ProblemData, detail string) *protocol.SegmentOutcome {
	result := &protocol.SegmentOutcome{Type: protocol.SegmentOutcomeType(outcome)}
	if outcome == rundomain.Failed || outcome == rundomain.Lost {
		result.Error = problem
	} else {
		result.Detail = detail
	}
	return result
}

func itemRecord(sessionID string, item protocol.Item, ordinal int) (transcript.Record, error) {
	body, err := json.Marshal(item)
	if err != nil {
		return transcript.Record{}, fmt.Errorf("runflow: encode item: %w", err)
	}
	createdAt := item.CreatedAt
	if createdAt.IsZero() {
		createdAt = item.StartedAt
	}
	if createdAt.IsZero() {
		return transcript.Record{}, errors.New("runflow: item has no occurrence time")
	}
	return transcript.Record{
		ID: item.ID, SessionID: sessionID, RunID: item.RunID,
		Ordinal: ordinal, Body: body,
		SearchText: transcriptflow.SearchableItem(item),
		CreatedAt: createdAt,
	}, nil
}

func profile(capabilities *protocol.ClientCapabilities) protocol.RunProtocolProfile {
	result := protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}}
	if capabilities == nil {
		return result
	}
	result.InterruptTypes = append(result.InterruptTypes, capabilities.InterruptTypes...)
	if capabilities.Features[protocol.FeatureSubagents].Enabled {
		result.RequiredFeatures = append(result.RequiredFeatures, protocol.RunProtocolFeatureSubagents)
	}
	return result
}

func runLimits(request protocol.StartRunRequest) *protocol.RunLimits {
	if request.MaxTotalTokens == 0 && request.MaxSteps == 0 && request.MaxBudgetUSD == 0 {
		return nil
	}
	return &protocol.RunLimits{
		MaxTotalTokens: request.MaxTotalTokens, MaxSteps: request.MaxSteps, MaxBudgetUSD: request.MaxBudgetUSD,
	}
}

func (service *Service) selection(ctx context.Context, request protocol.StartRunRequest, stored session.Session) (string, string, error) {
	if (request.Provider == "") != (request.Model == "") {
		return "", "", fmt.Errorf("%w: provider and model must be set together", protocol.ErrInvalidParams)
	}
	if request.Provider != "" {
		return request.Provider, request.Model, nil
	}
	if selection := stored.Selection(); !selection.Empty() {
		return selection.Provider(), selection.Model(), nil
	}
	return service.models.DefaultSelection(ctx)
}

func projectRunLookup(err error) error {
	if errors.Is(err, rundomain.ErrNotFound) {
		return protocol.ErrRunNotFound
	}
	return err
}

func (service *Service) runLock(runID string) *sync.Mutex {
	service.mu.Lock()
	defer service.mu.Unlock()
	lock := service.locks[runID]
	if lock == nil {
		lock = &sync.Mutex{}
		service.locks[runID] = lock
	}
	return lock
}

func encodeItemCursor(cursor transcript.Cursor, request protocol.ListItemsRequest) string {
	order, scopeType, scopeID, descendants := itemCursorIdentity(request)
	value := strings.Join([]string{
		order,
		scopeType,
		scopeID,
		descendants,
		cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		cursor.RunID,
		strconv.Itoa(cursor.Ordinal),
	}, "\n")
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeItemCursor(value string, request protocol.ListItemsRequest) (*transcript.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "\n", 7)
	if len(parts) != 7 || parts[5] == "" {
		return nil, errors.New("invalid item cursor")
	}
	order, scopeType, scopeID, descendants := itemCursorIdentity(request)
	if parts[0] != order || parts[1] != scopeType || parts[2] != scopeID || parts[3] != descendants {
		return nil, errors.New("item cursor belongs to another query")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[4])
	if err != nil {
		return nil, err
	}
	ordinal, err := strconv.Atoi(parts[6])
	if err != nil || ordinal < 0 {
		return nil, errors.New("invalid item cursor ordinal")
	}
	return &transcript.Cursor{
		CreatedAt: createdAt,
		RunID:     parts[5],
		Ordinal:   ordinal,
	}, nil
}

func itemCursorIdentity(request protocol.ListItemsRequest) (string, string, string, string) {
	order := string(request.Order)
	if order == "" {
		order = string(protocol.ItemOrderAsc)
	}
	scopeID := request.Scope.SessionID
	if request.Scope.Type == protocol.ItemScopeRun {
		scopeID = request.Scope.RunID
	}
	descendants := "0"
	if request.Scope.IncludeDescendants {
		descendants = "1"
	}
	return order, string(request.Scope.Type), scopeID, descendants
}

func encodeRunCursor(cursor rundomain.Cursor, request protocol.ListRunsRequest) string {
	sessionID, descendants, statuses := runCursorIdentity(request)
	value := strings.Join([]string{
		sessionID,
		descendants,
		statuses,
		cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		cursor.ID,
	}, "\n")
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeRunCursor(value string, request protocol.ListRunsRequest) (*rundomain.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "\n", 5)
	if len(parts) != 5 || parts[4] == "" {
		return nil, errors.New("invalid cursor")
	}
	sessionID, descendants, statuses := runCursorIdentity(request)
	if parts[0] != sessionID || parts[1] != descendants || parts[2] != statuses {
		return nil, errors.New("run cursor belongs to another query")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[3])
	if err != nil {
		return nil, err
	}
	return &rundomain.Cursor{CreatedAt: createdAt, ID: parts[4]}, nil
}

func runCursorIdentity(request protocol.ListRunsRequest) (string, string, string) {
	descendants := "0"
	if request.IncludeDescendants {
		descendants = "1"
	}
	statuses := make([]string, len(request.Statuses))
	for index, status := range request.Statuses {
		statuses[index] = string(status)
	}
	slices.Sort(statuses)
	return request.SessionID, descendants, strings.Join(statuses, ",")
}

func (service *Service) Close() {
	service.mu.Lock()
	if service.closing {
		service.mu.Unlock()
		return
	}
	service.closing = true
	service.cancel()
	for _, active := range service.active {
		active.cancel()
	}
	service.mu.Unlock()
	service.hub.Close()
	service.tasks.Wait()
}
