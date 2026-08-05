package runtime

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/agent/planning"
)

const compiledDefinitionFormat = 1

// Deployment is the immutable runtime-owned result of crossing the deployment
// boundary. Runtime planning never reads caller-owned Agent slices, goals,
// conditions, or action metadata after this value has been compiled.
//
// Executable functions remain delegated to the supplied Action/Condition and
// StuckPolicy values: Go cannot copy closure semantics. Their implementation
// identity is therefore outside the structural digest: callers must ensure the
// executable implementation associated with a DeploymentRef is compatible.
type Deployment struct {
	agent      *core.Agent
	descriptor core.AgentDescriptor
	ref        core.DeploymentRef
	definition []byte
}

// deploymentCompiler owns the immutable-definition snapshot and canonical
// encoding policy.
type deploymentCompiler struct{}

// Ref returns the portable value identity of this deployment.
func (d *Deployment) Ref() core.DeploymentRef {
	if d == nil {
		return core.DeploymentRef{}
	}
	return d.ref
}

// Descriptor returns the immutable, non-executable declaration compiled into
// this deployment.
func (d *Deployment) Descriptor() core.AgentDescriptor {
	if d == nil {
		return core.AgentDescriptor{}
	}
	return d.descriptor
}

func (c deploymentCompiler) compile(source *core.Agent) (*Deployment, error) {
	agent, err := c.snapshot(source)
	if err != nil {
		return nil, err
	}
	if err := validateAgentDefinition(agent); err != nil {
		return nil, fmt.Errorf("compile deployment %q: %w", agent.Name(), err)
	}
	return c.compileSnapshot(agent)
}

func (c deploymentCompiler) snapshot(source *core.Agent) (agent *core.Agent, err error) {
	if source == nil {
		return nil, errors.New("compile deployment: agent is nil")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			agent = nil
			err = panicerr.New("compile deployment: snapshot agent definition panicked", recovered)
		}
	}()
	return c.cloneAgent(source), nil
}

// compileSnapshot encodes a frozen definition that has already crossed the
// complete deployment validation boundary.
func (c deploymentCompiler) compileSnapshot(agent *core.Agent) (*Deployment, error) {
	definition, err := c.canonicalDefinition(agent)
	if err != nil {
		return nil, fmt.Errorf("compile deployment %q: %w", agent.Name(), err)
	}
	sum := sha256.Sum256(definition)
	ref := core.DeploymentRef{
		Name:   agent.Name(),
		Digest: hex.EncodeToString(sum[:]),
	}
	ref.Version = agent.Version()
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("compile deployment %q: %w", agent.Name(), err)
	}
	return &Deployment{
		agent:      agent,
		descriptor: agent.Descriptor(),
		ref:        ref,
		definition: slices.Clone(definition),
	}, nil
}

func (e *Engine) compileAgent(source *core.Agent) (*Deployment, error) {
	compiler := deploymentCompiler{}
	agent, err := compiler.snapshot(source)
	if err != nil {
		return nil, err
	}
	if err := e.validateForDeploy(agent); err != nil {
		return nil, err
	}
	return compiler.compileSnapshot(agent)
}

func validateAgentDefinition(agent *core.Agent) error {
	var problems []error
	if err := agent.Validate(); err != nil {
		problems = append(problems, err)
	}
	if _, err := planning.DomainForAgent(agent); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

func (c deploymentCompiler) cloneAgent(source *core.Agent) *core.Agent {
	if source == nil {
		return nil
	}

	actions := source.Actions()
	config := core.AgentConfig{
		Name:          source.Name(),
		Description:   source.Description(),
		Version:       source.Version(),
		StuckPolicy:   source.StuckPolicy(),
		Actions:       make([]core.Action, len(actions)),
		Goals:         source.Goals(),
		Conditions:    make([]core.Condition, len(source.Conditions())),
		SnapshotState: source.SnapshotState(),
		PlannerName:   source.PlannerName(),
	}

	for i, action := range actions {
		if nilvalue.Is(action) {
			continue
		}
		config.Actions[i] = c.freezeAction(action)
	}

	for i, condition := range source.Conditions() {
		if nilvalue.Is(condition) {
			continue
		}
		config.Conditions[i] = frozenCondition{
			delegate: condition,
			name:     condition.Name(),
			cost:     condition.Cost(),
		}
	}

	return core.NewAgent(config)
}

type frozenAction struct {
	delegate core.Action
	metadata core.ActionMetadata
}

func (a frozenAction) Metadata() core.ActionMetadata {
	return a.metadata.Clone()
}

func (a frozenAction) Execute(ctx context.Context, process *core.ProcessContext) (core.ActionStatus, error) {
	return a.delegate.Execute(ctx, process)
}

type frozenCondition struct {
	delegate core.Condition
	name     string
	cost     float64
}

func (c frozenCondition) Name() string  { return c.name }
func (c frozenCondition) Cost() float64 { return c.cost }

func (c frozenCondition) Evaluate(ctx context.Context, environment *core.ConditionEnv) core.Truth {
	return c.delegate.Evaluate(ctx, environment)
}

func (c deploymentCompiler) freezeAction(action core.Action) frozenAction {
	return frozenAction{delegate: action, metadata: action.Metadata().Clone()}
}

type canonicalDefinition struct {
	Format        int                  `json:"format"`
	Name          string               `json:"name"`
	Description   string               `json:"description,omitempty"`
	Version       string               `json:"version,omitempty"`
	Planner       string               `json:"planner,omitempty"`
	Actions       []canonicalAction    `json:"actions"`
	Goals         []canonicalGoal      `json:"goals"`
	Conditions    []canonicalCondition `json:"conditions,omitempty"`
	SnapshotState []canonicalBinding   `json:"snapshot_state,omitempty"`
	StuckPolicy   string               `json:"stuck_policy,omitempty"`
}

type canonicalAction struct {
	Name              string             `json:"name"`
	Description       string             `json:"description,omitempty"`
	Implementation    string             `json:"implementation"`
	Inputs            []canonicalBinding `json:"inputs,omitempty"`
	Outputs           []canonicalBinding `json:"outputs,omitempty"`
	Preconditions     map[string]string  `json:"preconditions,omitempty"`
	Effects           map[string]string  `json:"effects,omitempty"`
	Repeatable        bool               `json:"can_rerun,omitempty"`
	ToolGroups        []string           `json:"tool_groups,omitempty"`
	CostConfigured    bool               `json:"cost_configured"`
	ValueConfigured   bool               `json:"value_configured"`
	ClearWorkingState bool               `json:"clear_working_state,omitempty"`
}

type canonicalBinding struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type canonicalGoal struct {
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	Preconditions []string           `json:"pre,omitempty"`
	Inputs        []canonicalBinding `json:"inputs,omitempty"`
}

type canonicalCondition struct {
	Name           string  `json:"name"`
	Cost           float64 `json:"cost"`
	Implementation string  `json:"implementation"`
}

func (c deploymentCompiler) canonicalDefinition(agent *core.Agent) ([]byte, error) {
	definition := canonicalDefinition{
		Format:        compiledDefinitionFormat,
		Name:          agent.Name(),
		Description:   agent.Description(),
		Planner:       planning.EffectivePlannerName(agent.PlannerName()),
		Actions:       make([]canonicalAction, 0, len(agent.Actions())),
		Goals:         make([]canonicalGoal, 0, len(agent.Goals())),
		Conditions:    make([]canonicalCondition, 0, len(agent.Conditions())),
		SnapshotState: c.canonicalSnapshotState(agent.SnapshotState()),
		StuckPolicy:   c.typeName(agent.StuckPolicy()),
	}
	definition.Version = agent.Version()

	for _, action := range agent.Actions() {
		metadata := action.Metadata()
		definition.Actions = append(definition.Actions, canonicalAction{
			Name:              metadata.Name,
			Description:       metadata.Description,
			Implementation:    c.actionImplementation(action),
			Inputs:            c.canonicalBindings(metadata.Inputs),
			Outputs:           c.canonicalBindings(metadata.Outputs),
			Preconditions:     c.canonicalConditions(metadata.Preconditions),
			Effects:           c.canonicalConditions(metadata.Effects),
			Repeatable:        metadata.Repeatable,
			ToolGroups:        slices.Clone(metadata.ToolGroups),
			CostConfigured:    metadata.Cost != nil,
			ValueConfigured:   metadata.Value != nil,
			ClearWorkingState: metadata.ClearWorkingState,
		})
	}

	for _, goal := range agent.Goals() {
		definition.Goals = append(definition.Goals, c.canonicalGoal(goal))
	}

	for _, condition := range agent.Conditions() {
		definition.Conditions = append(definition.Conditions, canonicalCondition{
			Name:           condition.Name(),
			Cost:           condition.Cost(),
			Implementation: c.conditionImplementation(condition),
		})
	}

	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, fmt.Errorf("encode canonical definition: %w", err)
	}
	return encoded, nil
}

func (c deploymentCompiler) canonicalGoal(goal *core.Goal) canonicalGoal {
	return canonicalGoal{
		Name:          goal.Name(),
		Description:   goal.Description(),
		Preconditions: c.normalizedStrings(goal.RequiredConditions()),
		Inputs:        c.canonicalBindings(goal.Inputs()),
	}
}

func (c deploymentCompiler) canonicalBindings(bindings []core.Binding) []canonicalBinding {
	if len(bindings) == 0 {
		return nil
	}
	canonical := make([]canonicalBinding, len(bindings))
	for i, binding := range bindings {
		name := binding.Name
		if name == "" {
			name = core.DefaultBindingName
		}
		canonical[i] = canonicalBinding{Name: name, Type: binding.Type}
	}
	return canonical
}

func (c deploymentCompiler) canonicalSnapshotState(bindings []core.Binding) []canonicalBinding {
	canonical := c.canonicalBindings(bindings)
	slices.SortFunc(canonical, func(left, right canonicalBinding) int {
		if order := cmp.Compare(left.Name, right.Name); order != 0 {
			return order
		}
		return cmp.Compare(left.Type, right.Type)
	})
	return canonical
}

func (c deploymentCompiler) canonicalConditions(conditions core.ConditionSet) map[string]string {
	if len(conditions) == 0 {
		return nil
	}
	canonical := make(map[string]string, len(conditions))
	for name, truth := range conditions {
		canonical[name] = truth.String()
	}
	return canonical
}

func (c deploymentCompiler) normalizedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := slices.Clone(values)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func (c deploymentCompiler) actionImplementation(action core.Action) string {
	for {
		frozen, ok := action.(frozenAction)
		if !ok {
			return c.typeName(action)
		}
		action = frozen.delegate
	}
}

func (c deploymentCompiler) conditionImplementation(condition core.Condition) string {
	for {
		frozen, ok := condition.(frozenCondition)
		if !ok {
			return c.typeName(condition)
		}
		condition = frozen.delegate
	}
}

func (c deploymentCompiler) typeName(value any) string {
	if value == nil {
		return ""
	}
	typeOf := reflect.TypeOf(value)
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf.PkgPath() == "" {
		return typeOf.String()
	}
	return typeOf.PkgPath() + "." + typeOf.Name()
}
