package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type SessionStatus string

const (
	SessionRunning SessionStatus = "running"
	SessionWaiting SessionStatus = "waiting"
	SessionIdle    SessionStatus = "idle"
)

type Session struct {
	ID        string
	Title     string
	Status    SessionStatus
	Provider  string
	Model     string
	Workspace workspace.Workspace
	CreatedAt time.Time
	UpdatedAt time.Time
	Favorite  bool
	Revision  uint64
}

// Equal reports whether two session projections carry the same durable state.
func (s Session) Equal(other Session) bool {
	return s.ID == other.ID && s.Title == other.Title && s.Status == other.Status &&
		s.Provider == other.Provider && s.Model == other.Model && s.Workspace == other.Workspace &&
		s.CreatedAt.Equal(other.CreatedAt) && s.UpdatedAt.Equal(other.UpdatedAt) &&
		s.Favorite == other.Favorite && s.Revision == other.Revision
}

func (s Session) Validate() error {
	var problems []error
	if strings.TrimSpace(s.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if err := s.Workspace.Validate(); err != nil {
		problems = append(problems, err)
	}
	if s.Status != SessionRunning && s.Status != SessionWaiting && s.Status != SessionIdle {
		problems = append(problems, fmt.Errorf("status %q is invalid", s.Status))
	}
	if err := (ModelRef{Provider: s.Provider, Model: s.Model}).Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	return nil
}

// ModelRef is one complete provider/model identity. Provider and model always
// move together so callers cannot create a session update that the Runtime has
// to guess how to complete.
type ModelRef struct {
	Provider string
	Model    string
}

const modelRefSeparator = "/"

func NewModelRef(provider, model string) (ModelRef, error) {
	ref := ModelRef{Provider: strings.TrimSpace(provider), Model: strings.TrimSpace(model)}
	if err := ref.Validate(); err != nil {
		return ModelRef{}, err
	}
	return ref, nil
}

func ParseModelRef(value string) (ModelRef, error) {
	provider, model, found := strings.Cut(strings.TrimSpace(value), modelRefSeparator)
	if !found {
		return ModelRef{}, fmt.Errorf("model identity must use provider%smodel form", modelRefSeparator)
	}
	return NewModelRef(provider, model)
}

func (m ModelRef) Validate() error {
	if strings.TrimSpace(m.Provider) == "" || strings.TrimSpace(m.Model) == "" {
		return errors.New("model identity requires provider and model")
	}
	if m.Provider != strings.TrimSpace(m.Provider) || m.Model != strings.TrimSpace(m.Model) {
		return errors.New("model identity must not have surrounding whitespace")
	}
	if strings.Contains(m.Provider, modelRefSeparator) {
		return fmt.Errorf("model identity provider must not contain %q", modelRefSeparator)
	}
	return nil
}

func (m ModelRef) String() string { return m.Provider + modelRefSeparator + m.Model }

type SessionQuery struct {
	Cursor    string
	Limit     int
	Search    string
	Workspace string
}

// Normalize returns one exact session-catalog query. Search text and workspace
// input are presentation values, while a non-empty workspace is an absolute
// identity because filtering happens client-side against canonical responses.
func (s SessionQuery) Normalize() (SessionQuery, error) {
	if s.Limit < 0 {
		return SessionQuery{}, errors.New("session query: limit cannot be negative")
	}
	s.Search = strings.TrimSpace(s.Search)
	s.Workspace = strings.TrimSpace(s.Workspace)
	if err := (workspace.ResolveRequest{Path: s.Workspace}).Validate(); err != nil {
		return SessionQuery{}, fmt.Errorf("session query: %w", err)
	}
	return s, nil
}

type SessionPage struct {
	Items      []Session
	NextCursor string
}

func (s SessionPage) Validate() error {
	seen := make(map[string]struct{}, len(s.Items))
	for index, session := range s.Items {
		if err := session.Validate(); err != nil {
			return fmt.Errorf("session page item %d: %w", index+1, err)
		}
		if _, duplicate := seen[session.ID]; duplicate {
			return fmt.Errorf("session page repeats id %q", session.ID)
		}
		seen[session.ID] = struct{}{}
	}
	return nil
}

// SessionSnapshot is the cold-read projection the CLI restores. Transcript,
// Runs, and Plan are durable values, never reconstructed from a historical
// event stream. Runs contains roots and descendants in creation order.
type SessionSnapshot struct {
	Session      Session
	Transcript   []Block
	Runs         []Run
	Plan         []PlanItem
	PlanRevision uint64
	Interactions []Interaction
}

// LatestRun returns the most recently created root run.
func (s SessionSnapshot) LatestRun() (Run, bool) {
	for _, run := range slices.Backward(s.Runs) {
		if run.Lineage.IsRoot() {
			return run.Clone(), true
		}
	}
	return Run{}, false
}

// ActiveRun returns the sole running or waiting root run, when one exists.
func (s SessionSnapshot) ActiveRun() (Run, bool) {
	for _, run := range slices.Backward(s.Runs) {
		if run.Lineage.IsRoot() && run.Status != RunStatusFinished {
			return run.Clone(), true
		}
	}
	return Run{}, false
}

func (s SessionSnapshot) RunByID(id string) (Run, bool) {
	for _, run := range s.Runs {
		if run.ID == id {
			return run.Clone(), true
		}
	}
	return Run{}, false
}

// LastAssistantText returns the latest durable non-empty assistant response.
func (s SessionSnapshot) LastAssistantText() (string, error) {
	for _, block := range slices.Backward(s.Transcript) {
		if block.Kind == BlockAssistant && strings.TrimSpace(block.Text) != "" {
			return strings.TrimSpace(block.Text), nil
		}
	}
	return "", errors.New("the session has no assistant response to copy")
}

func (s SessionSnapshot) Validate() error {
	if err := s.Session.Validate(); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	transcript, err := s.validateTranscript()
	if err != nil {
		return err
	}
	runs, err := s.validateRuns()
	if err != nil {
		return err
	}
	if err := s.validateReferences(transcript, runs); err != nil {
		return err
	}
	if err := validatePlan(s.Plan); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	if s.PlanRevision == 0 && len(s.Plan) != 0 {
		return errors.New("session snapshot: unwritten plan contains items")
	}
	return s.validateLifecycle(transcript, runs)
}

type snapshotTranscript struct {
	byIdentity map[string]Block
	running    []Block
}

func (s SessionSnapshot) validateTranscript() (snapshotTranscript, error) {
	indexed := snapshotTranscript{byIdentity: make(map[string]Block, len(s.Transcript))}
	for i, block := range s.Transcript {
		if err := block.validateLifecycle(block.Status != BlockStatusRunning); err != nil {
			return snapshotTranscript{}, fmt.Errorf("session snapshot: transcript block %d: %w", i+1, err)
		}
		identity := blockIdentity(block.RunID, block.ID)
		if _, duplicate := indexed.byIdentity[identity]; duplicate {
			return snapshotTranscript{}, fmt.Errorf("session snapshot: transcript repeats block %q in run %q", block.ID, block.RunID)
		}
		indexed.byIdentity[identity] = block
		if block.Status != BlockStatusRunning {
			continue
		}
		if block.Kind != BlockTool {
			return snapshotTranscript{}, fmt.Errorf("session snapshot: transcript block %d: only a tool call can be durably running", i+1)
		}
		indexed.running = append(indexed.running, block)
	}
	return indexed, nil
}

type snapshotRuns struct {
	byID        map[string]Run
	position    map[string]int
	activeIndex int
}

func (s SessionSnapshot) validateRuns() (snapshotRuns, error) {
	indexed := snapshotRuns{
		byID: make(map[string]Run, len(s.Runs)), position: make(map[string]int, len(s.Runs)),
		activeIndex: -1,
	}
	lastRootIndex := -1
	for i, run := range s.Runs {
		if err := run.Validate(); err != nil {
			return snapshotRuns{}, fmt.Errorf("session snapshot: run %d: %w", i+1, err)
		}
		if run.SessionID != s.Session.ID {
			return snapshotRuns{}, fmt.Errorf("session snapshot: run %s belongs to session %s", run.ID, run.SessionID)
		}
		if _, duplicate := indexed.byID[run.ID]; duplicate {
			return snapshotRuns{}, fmt.Errorf("session snapshot: repeats run %q", run.ID)
		}
		indexed.byID[run.ID] = run
		indexed.position[run.ID] = i
		if !run.Lineage.IsRoot() {
			continue
		}
		lastRootIndex = i
		if run.Status == RunStatusFinished {
			continue
		}
		if indexed.activeIndex >= 0 {
			return snapshotRuns{}, errors.New("session snapshot: more than one root run is active")
		}
		indexed.activeIndex = i
	}
	if indexed.activeIndex >= 0 && indexed.activeIndex != lastRootIndex {
		return snapshotRuns{}, errors.New("session snapshot: active run is not the latest root run")
	}
	for i, run := range s.Runs {
		if run.Lineage.IsRoot() {
			continue
		}
		parent, parentExists := indexed.byID[run.Lineage.ParentRunID]
		root, rootExists := indexed.byID[run.Lineage.RootRunID]
		if !parentExists || !rootExists || !root.Lineage.IsRoot() {
			return snapshotRuns{}, fmt.Errorf("session snapshot: child run %s has an incomplete lineage", run.ID)
		}
		parentRootID := parent.Lineage.RootRunID
		if parent.Lineage.IsRoot() {
			parentRootID = parent.ID
		}
		if parentRootID != run.Lineage.RootRunID {
			return snapshotRuns{}, fmt.Errorf("session snapshot: child run %s crosses run trees", run.ID)
		}
		if indexed.position[parent.ID] >= i {
			return snapshotRuns{}, fmt.Errorf("session snapshot: child run %s precedes parent %s", run.ID, parent.ID)
		}
		if root.Status == RunStatusFinished && run.Status != RunStatusFinished {
			return snapshotRuns{}, fmt.Errorf("session snapshot: child run %s outlives finished root %s", run.ID, root.ID)
		}
		if root.Status == RunStatusRunning && run.Status == RunStatusWaiting {
			return snapshotRuns{}, fmt.Errorf("session snapshot: child run %s is waiting beneath running root %s", run.ID, root.ID)
		}
		if root.Status == RunStatusWaiting && run.Status == RunStatusRunning {
			return snapshotRuns{}, fmt.Errorf("session snapshot: child run %s is running beneath waiting root %s", run.ID, root.ID)
		}
	}
	return indexed, nil
}

func (s SessionSnapshot) validateReferences(transcript snapshotTranscript, runs snapshotRuns) error {
	for _, block := range s.Transcript {
		if _, exists := runs.byID[block.RunID]; !exists {
			return fmt.Errorf("session snapshot: block %s references unknown run %s", block.ID, block.RunID)
		}
	}
	if runs.activeIndex < 0 {
		return nil
	}
	for _, block := range transcript.running {
		run := runs.byID[block.RunID]
		rootID := run.Lineage.RootRunID
		if run.Lineage.IsRoot() {
			rootID = run.ID
		}
		active := s.Runs[runs.activeIndex]
		if rootID != active.ID || run.Status == RunStatusFinished {
			return fmt.Errorf("session snapshot: running block %s belongs to inactive run %s", block.ID, block.RunID)
		}
	}
	return nil
}

func (s SessionSnapshot) validateLifecycle(transcript snapshotTranscript, runs snapshotRuns) error {
	if runs.activeIndex < 0 {
		return s.validateIdleLifecycle(transcript)
	}
	active := s.Runs[runs.activeIndex]
	switch active.Status {
	case RunStatusRunning:
		if s.Session.Status != SessionRunning {
			return fmt.Errorf("session snapshot: running run has session status %s", s.Session.Status)
		}
		if len(s.Interactions) != 0 {
			return errors.New("session snapshot: running run carries pending interactions")
		}
	case RunStatusWaiting:
		return s.validateWaitingLifecycle(active, transcript, runs)
	}
	return nil
}

func (s SessionSnapshot) validateIdleLifecycle(transcript snapshotTranscript) error {
	if len(transcript.running) != 0 {
		return errors.New("session snapshot: idle session carries a running transcript block")
	}
	if len(s.Interactions) != 0 {
		return errors.New("session snapshot: idle session carries pending interactions")
	}
	if s.Session.Status != SessionIdle {
		return fmt.Errorf("session snapshot: session status is %s without an active run", s.Session.Status)
	}
	return nil
}

func (s SessionSnapshot) validateWaitingLifecycle(active Run, transcript snapshotTranscript, runs snapshotRuns) error {
	if s.Session.Status != SessionWaiting {
		return fmt.Errorf("session snapshot: waiting run has session status %s", s.Session.Status)
	}
	if err := ValidateInteractions(s.Interactions); err != nil {
		return fmt.Errorf("session snapshot: waiting run: %w", err)
	}
	for _, interaction := range s.Interactions {
		itemID := InteractionItemID(interaction)
		runID := InteractionRunID(interaction)
		run, runExists := runs.byID[runID]
		block, exists := transcript.byIdentity[blockIdentity(runID, itemID)]
		rootID := run.Lineage.RootRunID
		if run.Lineage.IsRoot() {
			rootID = run.ID
		}
		if !runExists || rootID != active.ID || run.Status != RunStatusWaiting {
			return fmt.Errorf("session snapshot: waiting interrupt references inactive run %s", runID)
		}
		if !exists {
			return fmt.Errorf("session snapshot: waiting interrupt references unknown item %s", itemID)
		}
		if err := validateInteractionItem(interaction, block); err != nil {
			return fmt.Errorf("session snapshot: waiting run: %w", err)
		}
	}
	return nil
}

func (c *Conversation) RestoreSnapshot(snapshot SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	next := NewConversation()
	next.blocks = cloneBlocks(snapshot.Transcript)
	next.plan = slices.Clone(snapshot.Plan)
	next.planRevision = snapshot.PlanRevision
	next.rebuildBlockIndex()
	for _, run := range snapshot.Runs {
		next.runs[run.ID] = run.Clone()
	}
	if active, ok := snapshot.ActiveRun(); ok {
		next.runID = active.ID
		next.segmentID = active.ActiveSegmentID
		next.usage = active.Usage.Clone()
		if active.Status == RunStatusWaiting {
			next.phase = ConversationWaiting
			next.interactions = CloneInteractions(snapshot.Interactions)
		} else {
			next.phase = ConversationRunning
			next.coldTail = true
		}
	} else if latest, ok := snapshot.LatestRun(); ok {
		next.runID = latest.ID
		next.usage = latest.Usage.Clone()
		next.outcome = latest.Outcome.Clone()
	}
	*c = *next
	return nil
}

// RestoreAttachedSnapshot restores a cold projection that was read after a
// cursorless subscription was attached. HeadEventID is the exact journal
// position preceding that stream; retaining it closes a second-disconnect gap
// before the first replayable event arrives.
func (c *Conversation) RestoreAttachedSnapshot(snapshot SessionSnapshot, stream SegmentStream) error {
	if err := stream.ValidateSubscription(); err != nil {
		return fmt.Errorf("restore attached snapshot: %w", err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok || active.Status != RunStatusRunning {
		return errors.New("restore attached snapshot: snapshot has no running run")
	}
	if active.ID != stream.RunID || active.ActiveSegmentID != stream.SegmentID {
		return fmt.Errorf(
			"restore attached snapshot: stream %s/%s does not match run %s/%s",
			stream.RunID, stream.SegmentID, active.ID, active.ActiveSegmentID,
		)
	}
	if err := c.RestoreSnapshot(snapshot); err != nil {
		return err
	}
	c.checkpoint = stream.HeadEventID
	c.reconciling = true
	return nil
}

type CreateSession struct {
	Title     string
	Workspace string
}

func (c CreateSession) Validate() error {
	if c.Title != "" && strings.TrimSpace(c.Title) == "" {
		return errors.New("session create: title is empty")
	}
	if err := (workspace.ResolveRequest{Path: strings.TrimSpace(c.Workspace)}).Validate(); err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	return nil
}

func (c CreateSession) ValidateResult(result Session) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if title := strings.TrimSpace(c.Title); title != "" && result.Title != title {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, title))
	}
	if path := strings.TrimSpace(c.Workspace); path != "" && result.Workspace.Path != path {
		problems = append(problems, fmt.Errorf("runtime returned workspace %q, want %q", result.Workspace.Path, path))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	return nil
}

type UpdateSession struct {
	SessionID        string
	Title            *string
	Workspace        *string
	Model            *ModelRef
	Favorite         *bool
	ExpectedRevision uint64
}

func (u UpdateSession) Validate() error {
	if strings.TrimSpace(u.SessionID) == "" {
		return errors.New("session update: session id is empty")
	}
	if u.Title == nil && u.Workspace == nil && u.Model == nil && u.Favorite == nil {
		return errors.New("session update: no fields are selected")
	}
	if u.Title != nil && strings.TrimSpace(*u.Title) == "" {
		return errors.New("session update: title is empty")
	}
	if u.Workspace != nil && strings.TrimSpace(*u.Workspace) == "" {
		return errors.New("session update: workspace is empty")
	}
	if u.Model != nil {
		if err := u.Model.Validate(); err != nil {
			return fmt.Errorf("session update: %w", err)
		}
	}
	return nil
}

// ValidateResult verifies that a successful update response represents the
// exact command the caller issued, rather than merely containing a valid but
// unrelated session projection.
func (u UpdateSession) ValidateResult(result Session) error {
	if err := u.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID != u.SessionID {
		problems = append(problems, fmt.Errorf("runtime returned session %s, want %s", result.ID, u.SessionID))
	}
	if result.Revision <= u.ExpectedRevision {
		problems = append(problems, fmt.Errorf("runtime returned revision %d after expected revision %d", result.Revision, u.ExpectedRevision))
	}
	if u.Title != nil && result.Title != strings.TrimSpace(*u.Title) {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, strings.TrimSpace(*u.Title)))
	}
	if u.Workspace != nil && result.Workspace.Path != strings.TrimSpace(*u.Workspace) {
		problems = append(problems, fmt.Errorf("runtime returned workspace %q, want %q", result.Workspace.Path, strings.TrimSpace(*u.Workspace)))
	}
	if u.Model != nil && (result.Provider != u.Model.Provider || result.Model != u.Model.Model) {
		problems = append(problems, fmt.Errorf("runtime returned model %q, want %q", (ModelRef{Provider: result.Provider, Model: result.Model}).String(), u.Model.String()))
	}
	if u.Favorite != nil && result.Favorite != *u.Favorite {
		problems = append(problems, fmt.Errorf("runtime returned favorite %t, want %t", result.Favorite, *u.Favorite))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("session update: %w", err)
	}
	return nil
}

type ForkSession struct {
	SessionID string
	FromRunID string
	Title     string
}

func (f ForkSession) Validate() error {
	if strings.TrimSpace(f.SessionID) == "" {
		return errors.New("session fork: session id is empty")
	}
	if f.FromRunID != "" && strings.TrimSpace(f.FromRunID) == "" {
		return errors.New("session fork: run id is empty")
	}
	if f.Title != "" && strings.TrimSpace(f.Title) == "" {
		return errors.New("session fork: title is empty")
	}
	return nil
}

func (f ForkSession) ValidateResult(result Session) error {
	if err := f.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID == strings.TrimSpace(f.SessionID) {
		problems = append(problems, fmt.Errorf("runtime returned source session %q", result.ID))
	}
	if title := strings.TrimSpace(f.Title); title != "" && result.Title != title {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, title))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("session fork: %w", err)
	}
	return nil
}

// DeleteSession identifies one idempotent session-deletion intent. CommandID
// is optional for one-shot callers; durable clients set it so an interrupted
// acknowledgement can be recovered without issuing a second mutation.
type DeleteSession struct {
	CommandID CommandID
	SessionID string
}

func cloneBlocks(blocks []Block) []Block {
	out := make([]Block, len(blocks))
	for i, block := range blocks {
		out[i] = block.Clone()
	}
	return out
}
