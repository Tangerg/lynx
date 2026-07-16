package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

const compiledDefinitionFormat = 1

// Deployment is the immutable runtime-owned result of crossing the deployment
// boundary. Runtime planning never reads caller-owned Agent slices, goals,
// conditions, or action metadata after this value has been compiled.
//
// Executable functions remain delegated to the supplied Action/Condition and
// StuckPolicy values: Go cannot copy closure semantics. Their implementation
// identity is therefore a semantic-version/host-build contract, not something
// reflection or a function pointer can prove.
type Deployment struct {
	source     *core.Agent
	agent      *core.Agent
	ref        core.DeploymentRef
	definition []byte
}

// Ref returns the durable value identity of this deployment.
func (d *Deployment) Ref() core.DeploymentRef {
	if d == nil {
		return core.DeploymentRef{}
	}
	return d.ref
}

// Agent returns the immutable compiled agent declaration. Agent, Goal,
// and built-in action metadata expose defensive collection snapshots, so the
// deployment can share one stable definition without cloning it per read.
func (d *Deployment) Agent() *core.Agent {
	if d == nil {
		return nil
	}
	return d.agent
}

func compileDeployment(source *core.Agent, buildID string) (*Deployment, error) {
	if source == nil {
		return nil, fmt.Errorf("compile deployment: agent is nil")
	}

	agent := cloneAgentDefinition(source)
	definition, err := canonicalAgentDefinition(agent, buildID)
	if err != nil {
		return nil, fmt.Errorf("compile deployment %q: %w", source.Name(), err)
	}
	sum := sha256.Sum256(definition)
	ref := core.DeploymentRef{
		Name:   agent.Name(),
		Digest: hex.EncodeToString(sum[:]),
	}
	ref.Version = agent.Version()
	return &Deployment{
		source:     source,
		agent:      agent,
		ref:        ref,
		definition: slices.Clone(definition),
	}, nil
}

func (e *Engine) compileAgent(source *core.Agent) (*Deployment, error) {
	if source == nil {
		return nil, fmt.Errorf("compile deployment: agent is nil")
	}
	if e.processStore != nil && source.Version() == "" && e.buildID == "" {
		return nil, fmt.Errorf("%w: agent %q is unversioned", ErrDurableIdentityRequired, source.Name())
	}
	return compileDeployment(source, e.buildID)
}

func cloneAgentDefinition(source *core.Agent) *core.Agent {
	if source == nil {
		return nil
	}

	actions := source.Actions()
	config := core.AgentConfig{
		Name:        source.Name(),
		Description: source.Description(),
		Version:     source.Version(),
		StuckPolicy: source.StuckPolicy(),
		Actions:     make([]core.Action, len(actions)),
		Goals:       source.Goals(),
		Conditions:  make([]core.Condition, len(source.Conditions())),
		PlannerName: source.PlannerName(),
	}

	for i, action := range actions {
		if action == nil {
			continue
		}
		config.Actions[i] = frozenAction{
			delegate: action,
			metadata: cloneRuntimeActionMetadata(action.Metadata()),
		}
	}

	for i, condition := range source.Conditions() {
		if condition == nil {
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
	return cloneRuntimeActionMetadata(a.metadata)
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

func cloneRuntimeActionMetadata(metadata core.ActionMetadata) core.ActionMetadata {
	metadata.Inputs = slices.Clone(metadata.Inputs)
	metadata.Outputs = slices.Clone(metadata.Outputs)
	metadata.Preconditions = maps.Clone(metadata.Preconditions)
	metadata.Effects = maps.Clone(metadata.Effects)
	if metadata.ToolGroups != nil {
		groups := make([]core.ToolGroupRequirement, len(metadata.ToolGroups))
		for i, group := range metadata.ToolGroups {
			groups[i] = group
			groups[i].AllowedPermissions = slices.Clone(group.AllowedPermissions)
		}
		metadata.ToolGroups = groups
	}
	return metadata
}

type canonicalDefinition struct {
	Format      int                  `json:"format"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Version     string               `json:"version,omitempty"`
	BuildID     string               `json:"build_id,omitempty"`
	Planner     string               `json:"planner,omitempty"`
	Actions     []canonicalAction    `json:"actions"`
	Goals       []canonicalGoal      `json:"goals"`
	Conditions  []canonicalCondition `json:"conditions,omitempty"`
	StuckPolicy string               `json:"stuck_policy,omitempty"`
}

type canonicalAction struct {
	Name              string                     `json:"name"`
	Description       string                     `json:"description,omitempty"`
	Implementation    string                     `json:"implementation"`
	Inputs            []canonicalBinding         `json:"inputs,omitempty"`
	Outputs           []canonicalBinding         `json:"outputs,omitempty"`
	Preconditions     map[string]string          `json:"preconditions,omitempty"`
	Effects           map[string]string          `json:"effects,omitempty"`
	Repeatable        bool                       `json:"can_rerun,omitempty"`
	Retry             canonicalRetryPolicy       `json:"retry"`
	ToolGroups        []canonicalToolRequirement `json:"tool_groups,omitempty"`
	CostConfigured    bool                       `json:"cost_configured"`
	ValueConfigured   bool                       `json:"value_configured"`
	ClearWorkingState bool                       `json:"clear_working_state,omitempty"`
}

type canonicalRetryPolicy struct {
	MaxAttempts int    `json:"max_attempts"`
	BaseDelayNS int64  `json:"base_delay_ns"`
	MaxDelayNS  int64  `json:"max_delay_ns"`
	Safety      string `json:"safety"`
}

type canonicalBinding struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type canonicalToolRequirement struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
}

type canonicalGoal struct {
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	Preconditions []string           `json:"pre,omitempty"`
	Inputs        []canonicalBinding `json:"inputs,omitempty"`
	Tags          []string           `json:"tags,omitempty"`
	Examples      []string           `json:"examples,omitempty"`
	Tool          *canonicalGoalTool `json:"tool,omitempty"`
}

type canonicalGoalTool struct {
	Standalone  bool            `json:"standalone,omitempty"`
	Description string          `json:"description,omitempty"`
	InputType   string          `json:"input_type,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type canonicalCondition struct {
	Name           string  `json:"name"`
	Cost           float64 `json:"cost"`
	Implementation string  `json:"implementation"`
}

func canonicalAgentDefinition(agent *core.Agent, buildID string) ([]byte, error) {
	definition := canonicalDefinition{
		Format:      compiledDefinitionFormat,
		Name:        agent.Name(),
		Description: agent.Description(),
		BuildID:     buildID,
		Planner:     agent.PlannerName(),
		Actions:     make([]canonicalAction, 0, len(agent.Actions())),
		Goals:       make([]canonicalGoal, 0, len(agent.Goals())),
		Conditions:  make([]canonicalCondition, 0, len(agent.Conditions())),
		StuckPolicy: stableTypeName(agent.StuckPolicy()),
	}
	definition.Version = agent.Version()
	if definition.Planner == "" {
		definition.Planner = "goap"
	}

	for _, action := range agent.Actions() {
		metadata := action.Metadata()
		definition.Actions = append(definition.Actions, canonicalAction{
			Name:           metadata.Name,
			Description:    metadata.Description,
			Implementation: stableActionImplementation(action),
			Inputs:         canonicalBindings(metadata.Inputs),
			Outputs:        canonicalBindings(metadata.Outputs),
			Preconditions:  canonicalEffects(metadata.Preconditions),
			Effects:        canonicalEffects(metadata.Effects),
			Repeatable:     metadata.Repeatable,
			Retry: canonicalRetryPolicy{
				MaxAttempts: metadata.Retry.MaxAttempts,
				BaseDelayNS: int64(metadata.Retry.BaseDelay),
				MaxDelayNS:  int64(metadata.Retry.MaxDelay),
				Safety:      metadata.Retry.Safety.String(),
			},
			ToolGroups:        canonicalToolGroups(metadata.ToolGroups),
			CostConfigured:    metadata.Cost != nil,
			ValueConfigured:   metadata.Value != nil,
			ClearWorkingState: metadata.ClearWorkingState,
		})
	}

	for _, goal := range agent.Goals() {
		canonical, err := canonicalizeGoal(goal)
		if err != nil {
			return nil, err
		}
		definition.Goals = append(definition.Goals, canonical)
	}

	for _, condition := range agent.Conditions() {
		definition.Conditions = append(definition.Conditions, canonicalCondition{
			Name:           condition.Name(),
			Cost:           condition.Cost(),
			Implementation: stableConditionImplementation(condition),
		})
	}

	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, fmt.Errorf("encode canonical definition: %w", err)
	}
	return encoded, nil
}

func canonicalizeGoal(goal *core.Goal) (canonicalGoal, error) {
	canonical := canonicalGoal{
		Name:          goal.Name(),
		Description:   goal.Description(),
		Preconditions: normalizedStrings(goal.RequiredConditions()),
		Inputs:        canonicalBindings(goal.Inputs()),
		Tags:          goal.Tags(),
		Examples:      goal.Examples(),
	}
	toolConfig := goal.Tool()
	if toolConfig == nil {
		return canonical, nil
	}

	inputType := toolConfig.InputType()
	if inputType == nil || inputType.Kind() == reflect.Interface {
		return canonicalGoal{}, fmt.Errorf("goal %q tool input type must not be an interface", goal.Name())
	}
	tool := &canonicalGoalTool{
		Standalone:  toolConfig.Standalone,
		Description: toolConfig.Description,
		InputType:   stableTypeName(reflect.Zero(inputType).Interface()),
	}
	schema, err := schemaFor(reflect.Zero(inputType).Interface())
	if err != nil {
		return canonicalGoal{}, fmt.Errorf("goal %q tool schema: %w", goal.Name(), err)
	}
	tool.InputSchema = json.RawMessage(schema)
	canonical.Tool = tool
	return canonical, nil
}

func canonicalBindings(bindings []core.Binding) []canonicalBinding {
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

func canonicalEffects(effects core.ConditionSet) map[string]string {
	if len(effects) == 0 {
		return nil
	}
	canonical := make(map[string]string, len(effects))
	for name, truth := range effects {
		canonical[name] = truth.String()
	}
	return canonical
}

func canonicalToolGroups(groups []core.ToolGroupRequirement) []canonicalToolRequirement {
	if len(groups) == 0 {
		return nil
	}
	canonical := make([]canonicalToolRequirement, len(groups))
	for i, group := range groups {
		permissions := make([]string, len(group.AllowedPermissions))
		for j, permission := range group.AllowedPermissions {
			permissions[j] = permission.String()
		}
		canonical[i] = canonicalToolRequirement{Role: group.Role, Permissions: normalizedStrings(permissions)}
	}
	return canonical
}

func normalizedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := slices.Clone(values)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func stableActionImplementation(action core.Action) string {
	for {
		frozen, ok := action.(frozenAction)
		if !ok {
			return stableTypeName(action)
		}
		action = frozen.delegate
	}
}

func stableConditionImplementation(condition core.Condition) string {
	for {
		frozen, ok := condition.(frozenCondition)
		if !ok {
			return stableTypeName(condition)
		}
		condition = frozen.delegate
	}
}

func stableTypeName(value any) string {
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
