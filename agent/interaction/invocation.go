package interaction

import (
	"context"
	"slices"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/core/chat"
)

type invocationContextKey uint8

const (
	modelInvocationContextKey invocationContextKey = iota
	toolInvocationContextKey
)

// ModelInvocation is the immutable execution attribution of one actual model
// call. It contains no Engine handle or Host metadata.
type ModelInvocation struct {
	relation              agent.ProcessRelation
	deploymentRef         agent.DeploymentRef
	effectID              agent.EffectID
	stepSequence          uint64
	modelCallSequence     uint32
	appliedSteerSignalIDs []agent.SignalID
}

// Relation returns the Process tree location that owns the model call.
func (invocation ModelInvocation) Relation() agent.ProcessRelation { return invocation.relation }

// DeploymentRef returns the exact Interaction binding that owns the model call.
func (invocation ModelInvocation) DeploymentRef() agent.DeploymentRef {
	return invocation.deploymentRef
}

// EffectID returns the stable model Effect identity.
func (invocation ModelInvocation) EffectID() agent.EffectID { return invocation.effectID }

// StepSequence returns the one-based Process Step that declared the model Effect.
func (invocation ModelInvocation) StepSequence() uint64 { return invocation.stepSequence }

// ModelCallSequence returns the one-based model call position in this Interaction.
func (invocation ModelInvocation) ModelCallSequence() uint32 {
	return invocation.modelCallSequence
}

// AppliedSteerSignalIDs returns the ordered identities of steer Signals whose
// messages were first made visible to this exact model request. The returned
// slice is independently owned. An empty slice means the request applied no new
// steer input; previously applied messages may still remain in WorkingContext.
func (invocation ModelInvocation) AppliedSteerSignalIDs() []agent.SignalID {
	return slices.Clone(invocation.appliedSteerSignalIDs)
}

// Valid reports whether invocation contains one complete model-call attribution.
func (invocation ModelInvocation) Valid() bool {
	return invocation.relation.Valid() && invocation.deploymentRef.Valid() &&
		invocation.effectID.Valid() && invocation.stepSequence > 0 &&
		invocation.modelCallSequence > 0 &&
		(len(invocation.appliedSteerSignalIDs) == 0 || validateSteerSignalIDs(invocation.appliedSteerSignalIDs) == nil)
}

// ModelInvocationFromContext returns the attribution installed only for the
// duration of an Interaction model call.
func ModelInvocationFromContext(ctx context.Context) (ModelInvocation, bool) {
	if ctx == nil {
		return ModelInvocation{}, false
	}
	invocation, present := ctx.Value(modelInvocationContextKey).(ModelInvocation)
	return invocation, present && invocation.Valid()
}

// ToolInvocation is the immutable execution attribution of one actual Tool
// call. ToolCall is the exact model request being executed.
type ToolInvocation struct {
	relation          agent.ProcessRelation
	deploymentRef     agent.DeploymentRef
	effectID          agent.EffectID
	stepSequence      uint64
	modelCallSequence uint32
	toolCallIndex     uint32
	toolCall          chat.ToolCall
}

// Relation returns the Process tree location that owns the Tool call.
func (invocation ToolInvocation) Relation() agent.ProcessRelation { return invocation.relation }

// DeploymentRef returns the exact Interaction binding that owns the Tool call.
func (invocation ToolInvocation) DeploymentRef() agent.DeploymentRef {
	return invocation.deploymentRef
}

// EffectID returns the stable Tool-batch Effect identity.
func (invocation ToolInvocation) EffectID() agent.EffectID { return invocation.effectID }

// StepSequence returns the one-based Process Step that declared the Tool batch.
func (invocation ToolInvocation) StepSequence() uint64 { return invocation.stepSequence }

// ModelCallSequence returns the one-based model call that requested the Tool.
func (invocation ToolInvocation) ModelCallSequence() uint32 {
	return invocation.modelCallSequence
}

// ToolCallIndex returns the zero-based ToolCall position in the model response.
func (invocation ToolInvocation) ToolCallIndex() uint32 { return invocation.toolCallIndex }

// ToolCall returns the exact model ToolCall value being executed.
func (invocation ToolInvocation) ToolCall() chat.ToolCall { return invocation.toolCall }

// Valid reports whether invocation contains one complete Tool-call attribution.
func (invocation ToolInvocation) Valid() bool {
	return invocation.relation.Valid() && invocation.deploymentRef.Valid() &&
		invocation.effectID.Valid() && invocation.stepSequence > 0 &&
		invocation.modelCallSequence > 0 && invocation.toolCall.Validate() == nil
}

// ToolInvocationFromContext returns the attribution installed only for the
// duration of the exact Interaction Tool call.
func ToolInvocationFromContext(ctx context.Context) (ToolInvocation, bool) {
	if ctx == nil {
		return ToolInvocation{}, false
	}
	invocation, present := ctx.Value(toolInvocationContextKey).(ToolInvocation)
	return invocation, present && invocation.Valid()
}

func modelInvocationFromRequest(
	request agent.EffectRequest,
	modelCallSequence uint32,
	appliedSteerSignalIDs []agent.SignalID,
) ModelInvocation {
	return ModelInvocation{
		relation: request.Relation(), deploymentRef: request.DeploymentRef(),
		effectID: request.ID(), stepSequence: request.StepSequence(),
		modelCallSequence:     modelCallSequence,
		appliedSteerSignalIDs: slices.Clone(appliedSteerSignalIDs),
	}
}

func toolInvocationFromRequest(
	request agent.EffectRequest,
	modelCallSequence uint32,
	toolCallIndex uint32,
	toolCall chat.ToolCall,
) ToolInvocation {
	return ToolInvocation{
		relation: request.Relation(), deploymentRef: request.DeploymentRef(),
		effectID: request.ID(), stepSequence: request.StepSequence(),
		modelCallSequence: modelCallSequence, toolCallIndex: toolCallIndex,
		toolCall: toolCall,
	}
}

func withModelInvocation(ctx context.Context, invocation ModelInvocation) context.Context {
	return context.WithValue(ctx, modelInvocationContextKey, invocation)
}

func withToolInvocation(ctx context.Context, invocation ToolInvocation) context.Context {
	return context.WithValue(ctx, toolInvocationContextKey, invocation)
}
