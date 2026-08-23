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
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/streamhub"
)

type Store interface {
	GetSession(context.Context, session.ID) (session.Session, error)
	GetRun(context.Context, string) (rundomain.Record, error)
	GetOpenRootRun(context.Context, string) (rundomain.Record, error)
	ListRunningRuns(context.Context) ([]rundomain.Record, error)
	ListRuns(context.Context, string, []rundomain.Status, bool, int, *rundomain.Cursor) (rundomain.Page, error)
	CreateRun(context.Context, rundomain.Record, *transcript.Record, *conversationdomain.Record, []rundomain.EventRecord) error
	CommitRun(context.Context, CommitWrite) error
	ListItems(context.Context, string, string) ([]transcript.Record, error)
	ListConversationMessages(context.Context, string) ([]conversationdomain.Record, error)
	ListRunEvents(context.Context, string, string, string) ([]rundomain.EventRecord, error)
	GetInterruptSet(context.Context, string) (protocol.PendingInterruptSet, error)
	CommitResume(context.Context, ResumeWrite) error
	CommitWait(context.Context, WaitWrite) error
	CommitSteer(context.Context, SteerWrite) error
	GetExecutorCheckpoint(context.Context, string) ([]byte, error)
}

type IDs interface{ New(string) (string, error) }

type Executor interface {
	Execute(context.Context, agentexec.Input) (agentexec.Output, error)
	Resume(context.Context, agentexec.ResumeInput) (agentexec.Output, error)
}

type ModelSelection interface {
	DefaultSelection(context.Context) (string, string, error)
}

type Checkpoints interface {
	Snapshot(context.Context, string, string, string) error
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
}

type Config struct {
	Store Store
	IDs IDs
	Executor Executor
	Models ModelSelection
	Checkpoints Checkpoints
	Hub *streamhub.Hub
	Lifetime context.Context
	Clock func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.IDs == nil || config.Executor == nil || config.Models == nil || config.Hub == nil || config.Checkpoints == nil {
		return nil, errors.New("runflow: store, ids, executor, models, checkpoints and hub are required")
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
		hub: config.Hub, now: clock, lifetime: lifetime, cancel: cancel,
		active: make(map[string]activeExecution), locks: make(map[string]*sync.Mutex),
	}, nil
}

type StartCommand struct {
	Request protocol.StartRunRequest
	Meta    protocol.RequestMeta
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

func (service *Service) Start(ctx context.Context, command StartCommand) (
	*protocol.StartRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	request := command.Request
	storedSession, err := service.store.GetSession(ctx, session.ID(request.SessionID))
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
	providerID, model, err := service.selection(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	runID, err := service.ids.New("run_")
	if err != nil {
		return nil, nil, err
	}
	segmentID, err := service.ids.New("seg_")
	if err != nil {
		return nil, nil, err
	}
	itemID, err := service.ids.New("itm_")
	if err != nil {
		return nil, nil, err
	}
	now := service.now().UTC()
	aggregate, err := rundomain.New(rundomain.Start{
		ID: runID, SessionID: request.SessionID, SegmentID: segmentID,
		Provider: providerID, Model: model, Now: now,
	})
	if err != nil {
		return nil, nil, err
	}
	facts := runFacts{
		Metrics: protocol.RunMetrics{}, Limits: runLimits(request),
		Profile: profile(command.Meta.ClientCapabilities), EventOrdinal: 2,
	}
	record, err := makeRecord(aggregate, facts)
	if err != nil {
		return nil, nil, err
	}
	userItem := protocol.Item{
		ID: itemID, RunID: runID, Status: protocol.ItemStatusCompleted,
		CreatedAt: now, Type: protocol.ItemTypeUserMessage, Content: request.Input,
	}
	opening, err := itemRecord(request.SessionID, userItem, 0)
	if err != nil {
		return nil, nil, err
	}
	presented, err := presentRecord(record)
	if err != nil {
		return nil, nil, err
	}
	events, persisted, err := service.startEvents(runID, segmentID, *presented, userItem, now)
	if err != nil {
		return nil, nil, err
	}
	conversation, nextMessageOrdinal, err := service.conversation(ctx, request.SessionID)
	if err != nil {
		return nil, nil, err
	}
	userMessage, err := agentexec.UserMessage(request.Input)
	if err != nil { return nil, nil, err }
	conversation = append(conversation, userMessage)
	messageBody, err := json.Marshal(userMessage)
	if err != nil { return nil, nil, err }
	openingMessage := conversationdomain.Record{SessionID: request.SessionID, RunID: runID, Ordinal: nextMessageOrdinal, Body: messageBody}
	if err := service.store.CreateRun(ctx, record, &opening, &openingMessage, persisted); err != nil {
		if existing, lookupErr := service.store.GetOpenRootRun(ctx, request.SessionID); lookupErr == nil {
			active, presentErr := presentRecord(existing)
			if presentErr == nil {
				return nil, nil, &ActiveRunError{Run: *active}
			}
		}
		return nil, nil, err
	}
	stream := service.hub.SubscribeRun(ctx, runID, segmentID, events)
	if !service.launchExecution(runID, segmentID, storedSession.Workspace().Path(), conversation) {
		service.settleUnlaunched(runID)
	}
	return &protocol.StartRunResponse{RunID: runID, SegmentID: segmentID, UserItemID: itemID}, stream, nil
}

func (service *Service) launchExecution(runID, segmentID, workspace string, conversation []agentexec.Message) bool {
	service.mu.Lock()
	if service.closing {
		service.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(service.lifetime)
	steers := make(chan agentexec.Steer, 32)
	service.active[runID] = activeExecution{cancel: cancel, steers: steers}
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
		if err != nil {
			return
		}
		output, executeErr := service.executor.Execute(ctx, agentexec.Input{
			Provider: record.Run.Provider(), Model: record.Run.Model(), Workspace: workspace,
			SessionID: record.Run.SessionID(), RunID: runID, Conversation: conversation, Steers: steers,
			MaxSteps: runMaxSteps(record.Body),
		})
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
	lock := service.runLock(runID)
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	record, err := service.store.GetRun(ctx, runID)
	if err != nil || record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() != segmentID {
		return
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return
	}
	now := service.now().UTC()
	mergeRunUsage(&facts.Metrics, output.Usage, output.ModelCalls)
	terminalProjection := executeErr == nil && output.Waiting == nil
	projectedFacts := facts
	projection, projectionErr := service.projectExecution(ctx, record, segmentID, output, &projectedFacts, terminalProjection)
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
		if errors.Is(executeErr, context.Canceled) && service.lifetime.Err() != nil {
			outcome = rundomain.Lost
			detail = lostRunDetail
			problem = &protocol.ProblemData{Type: protocol.ProblemRunLost, Detail: detail}
		} else {
			outcome = rundomain.Failed
			detail = "the agent execution failed"
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
	persisted, err := persistEvents(events, firstOrdinal)
	if err != nil {
		return
	}
	if err := service.store.CommitRun(ctx, CommitWrite{Run: record, ExpectedStatus: rundomain.Running, ExpectedSegmentID: segmentID, Items: items, Messages: projection.messages, ToolResults: projection.results, Events: persisted}); err != nil {
		return
	}
	for _, event := range events {
		service.hub.PublishRun(event)
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
	cursor, err := decodeRunCursor(request.Cursor)
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
		next = encodeRunCursor(*page.Next)
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
	replay, head, err := service.replay(ctx, request.RunID, request.SegmentID, afterEventID)
	if err != nil {
		return nil, nil, err
	}
	return &protocol.SubscribeRunResponse{
		RunID: request.RunID, SegmentID: request.SegmentID, HeadEventID: head,
	}, service.hub.SubscribeRun(ctx, request.RunID, request.SegmentID, replay), nil
}

func (service *Service) Cancel(ctx context.Context, request protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	lock := service.runLock(request.RunID)
	lock.Lock()
	defer lock.Unlock()
	record, err := service.store.GetRun(ctx, request.RunID)
	if err != nil {
		return nil, projectRunLookup(err)
	}
	if record.Run.Status() == rundomain.Finished {
		return nil, protocol.ErrRunFinished
	}
	expectedStatus := record.Run.Status()
	expectedSegmentID := record.Run.ActiveSegmentID()
	service.mu.Lock()
	if active, ok := service.active[request.RunID]; ok {
		active.cancel()
	}
	service.mu.Unlock()
	segmentID := record.Run.ActiveSegmentID()
	if segmentID == "" {
		segmentID, err = service.ids.New("seg_")
		if err != nil {
			return nil, err
		}
		if err := record.Run.Resume(segmentID, service.now()); err != nil {
			return nil, err
		}
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	storedSession, err := service.store.GetSession(ctx, session.ID(record.Run.SessionID()))
	if err != nil {
		return nil, err
	}
	if err := service.checkpoints.Snapshot(ctx, record.Run.SessionID(), storedSession.Workspace().Path(), request.RunID); err != nil {
		return nil, fmt.Errorf("runflow: checkpoint canceled run: %w", err)
	}
	if err := record.Run.Finish(segmentID, rundomain.Canceled, request.Reason, now); err != nil {
		return nil, err
	}
	event, err := service.event(request.RunID, segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamSegmentFinished,
		Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentCanceled, Detail: request.Reason},
		Metrics: &facts.Metrics,
	}, now)
	if err != nil {
		return nil, err
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return nil, err
	}
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal)
	if err != nil {
		return nil, err
	}
	if err := service.store.CommitRun(ctx, CommitWrite{Run: record, ExpectedStatus: expectedStatus, ExpectedSegmentID: expectedSegmentID, Events: persisted}); err != nil {
		return nil, err
	}
	service.hub.PublishRun(event)
	presented, err := presentRecord(record)
	if err != nil {
		return nil, err
	}
	resultType := protocol.CancelRunRoot
	if record.Run.ParentRunID() != "" {
		resultType = protocol.CancelRunChild
	}
	return &protocol.CancelRunResponse{Type: resultType, Run: *presented}, nil
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
		persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal)
		if err != nil { active.cancel(); return err }
		if err := service.store.CommitSteer(ctx, SteerWrite{Run: record, ExpectedSegmentID: request.ExpectedSegmentID, Item: storedItem, Message: storedMessage, Event: persisted[0]}); err != nil { active.cancel(); return err }
		service.hub.PublishRun(event)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-service.lifetime.Done():
		return protocol.ErrRunFinished
	}
}

func (service *Service) Items(ctx context.Context, request protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	var sessionID, runID string
	switch request.Scope.Type {
	case protocol.ItemScopeSession:
		sessionID = request.Scope.SessionID
	case protocol.ItemScopeRun:
		runID = request.Scope.RunID
	default:
		return nil, fmt.Errorf("%w: item scope is invalid", protocol.ErrInvalidParams)
	}
	records, err := service.store.ListItems(ctx, sessionID, runID)
	if err != nil {
		return nil, err
	}
	items := make([]protocol.Item, 0, len(records))
	runIDs := make(map[string]bool)
	for _, record := range records {
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return nil, fmt.Errorf("runflow: decode item: %w", err)
		}
		items = append(items, item)
		runIDs[item.RunID] = true
	}
	if request.Order == protocol.ItemOrderDesc {
		for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
			items[left], items[right] = items[right], items[left]
		}
	}
	runs := make([]protocol.RunSummary, 0, len(runIDs))
	for id := range runIDs {
		record, err := service.store.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		presented, err := presentRecord(record)
		if err != nil {
			return nil, err
		}
		runs = append(runs, presented.RunSummary)
	}
	return &protocol.ListItemsResponse{Page: *protocol.NewPage(items), Runs: runs}, nil
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
		var event protocol.RunEvent
		if err := json.Unmarshal(record.Body, &event); err != nil {
			return nil, "", fmt.Errorf("runflow: decode event: %w", err)
		}
		events = append(events, event)
		head = event.EventID
	}
	return events, head, nil
}

func (service *Service) startEvents(
	runID, segmentID string,
	run protocol.RunRef,
	item protocol.Item,
	now time.Time,
) ([]protocol.RunEvent, []rundomain.EventRecord, error) {
	firstID, err := service.ids.New("evt_")
	if err != nil {
		return nil, nil, err
	}
	secondID, err := service.ids.New("evt_")
	if err != nil {
		return nil, nil, err
	}
	events := []protocol.RunEvent{
		{RunID: runID, SegmentID: segmentID, EventID: firstID, Timestamp: now,
			Event: protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &run}},
		{RunID: runID, SegmentID: segmentID, EventID: secondID, Timestamp: now,
			Event: protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}},
	}
	persisted, err := persistEvents(events, 1)
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

func persistEvents(events []protocol.RunEvent, firstOrdinal int) ([]rundomain.EventRecord, error) {
	records := make([]rundomain.EventRecord, 0, len(events))
	for index, event := range events {
		body, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		records = append(records, rundomain.EventRecord{
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
	return &protocol.RunRef{
		RunSummary: summary, ActiveSegmentID: value.ActiveSegmentID(), Metrics: facts.Metrics,
		ContextTokens: facts.ContextTokens, Limits: facts.Limits, ProtocolProfile: facts.Profile,
	}, nil
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
	return transcript.Record{
		ID: item.ID, SessionID: sessionID, RunID: item.RunID,
		Ordinal: ordinal, Body: body, CreatedAt: item.CreatedAt,
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

func (service *Service) selection(ctx context.Context, request protocol.StartRunRequest) (string, string, error) {
	if (request.Provider == "") != (request.Model == "") {
		return "", "", fmt.Errorf("%w: provider and model must be set together", protocol.ErrInvalidParams)
	}
	if request.Provider != "" {
		return request.Provider, request.Model, nil
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

func encodeRunCursor(cursor rundomain.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "\n" + cursor.ID))
}

func decodeRunCursor(value string) (*rundomain.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "\n", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	return &rundomain.Cursor{CreatedAt: createdAt, ID: parts[1]}, nil
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
