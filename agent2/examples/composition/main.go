// Command composition demonstrates that direct Engine embedding and a
// cross-Strategy composed Agent use the same Definition/Execution/Process
// contracts. It uses deterministic local components and requires no network.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	localDeployment, err := newTextDeployment()
	if err != nil {
		return err
	}
	modelDeployment, err := newModelDeployment()
	if err != nil {
		return err
	}
	compositionDeployment, err := newCompositionDeployment(
		localDeployment.Reference(), modelDeployment.Reference(),
	)
	if err != nil {
		return err
	}
	resolver := deploymentResolver{
		localDeployment.Reference(): localDeployment,
		modelDeployment.Reference(): modelDeployment,
	}
	engine, err := agent.NewEngine(agent.EngineConfig{DeploymentResolver: resolver})
	if err != nil {
		return err
	}
	defer engine.Close()

	embeddedInput, _ := agent.EncodeInput(textInput{Text: "embedded"})
	embeddedResult, err := engine.Run(ctx, localDeployment, embeddedInput)
	if err != nil {
		return err
	}
	embedded, err := decodeCompleted[textOutput](embeddedResult)
	if err != nil {
		return err
	}

	compositionInput, _ := agent.EncodeInput(compositionInput{Prompt: "composition"})
	compositionResult, err := engine.Run(ctx, compositionDeployment, compositionInput)
	if err != nil {
		return err
	}
	composed, err := decodeCompleted[compositionOutput](compositionResult)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		output, "embedded: %s\ncomposed: %s | %s\n",
		embedded.Text, composed.Local, composed.Model,
	)
	return err
}

type textInput struct {
	Text string `json:"text"`
}

type textOutput struct {
	Text string `json:"text"`
}

type textDefinition struct{ descriptor agent.Descriptor }

func newTextDeployment() (agent.Deployment, error) {
	inputSchema, err := agent.SchemaFor[textInput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	outputSchema, err := agent.SchemaFor[textOutput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "example.uppercase", Description: "Return input text in uppercase.",
		Version: "1.0.0", InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return agent.Deployment{}, err
	}
	return agent.NewDeployment(agent.DeploymentConfig{
		Definition: &textDefinition{descriptor: descriptor}, Dispatcher: rejectingDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("example-uppercase-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-uppercase-configuration")),
	})
}

func (definition *textDefinition) Descriptor() agent.Descriptor { return definition.descriptor }

func (*textDefinition) Start(input agent.Input) (agent.Execution, error) {
	decoded, err := agent.DecodeInput[textInput](input)
	if err != nil {
		return nil, err
	}
	return &textExecution{Text: decoded.Text}, nil
}

func (*textDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "example.uppercase" || state.SchemaVersion() != 1 {
		return nil, agent.ErrInvalidExecutionState
	}
	var execution textExecution
	if err := json.Unmarshal(state.Payload(), &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

type textExecution struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

func (execution *textExecution) Step(context.Context, []agent.Signal) (agent.Transition, error) {
	if execution.Done {
		return agent.Transition{}, errors.New("uppercase execution already completed")
	}
	execution.Done = true
	value, _ := agent.EncodeOutput(textOutput{Text: strings.ToUpper(execution.Text)})
	return agent.Complete(0, value)
}

func (execution *textExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(execution)
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("example.uppercase", 1, payload)
}

func newModelDeployment() (agent.Deployment, error) {
	client, err := chatclient.New(compositionModel{}, chatclient.Config{})
	if err != nil {
		return agent.Deployment{}, err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "example.composition_model", Description: "Return one deterministic composition response.",
		Version: "1.0.0", MaxModelCalls: 1,
	})
	if err != nil {
		return agent.Deployment{}, err
	}
	dispatcher, err := interaction.NewDispatcher(interaction.DispatcherConfig{Client: client})
	if err != nil {
		return agent.Deployment{}, err
	}
	return agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("example-composition-model-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-composition-model-configuration")),
	})
}

type compositionInput struct {
	Prompt string `json:"prompt"`
}

type compositionOutput struct {
	Local string `json:"local"`
	Model string `json:"model"`
}

type compositionDefinition struct {
	descriptor agent.Descriptor
	local      agent.DeploymentRef
	model      agent.DeploymentRef
}

func newCompositionDeployment(local, model agent.DeploymentRef) (agent.Deployment, error) {
	inputSchema, err := agent.SchemaFor[compositionInput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	outputSchema, err := agent.SchemaFor[compositionOutput]()
	if err != nil {
		return agent.Deployment{}, err
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "example.composition", Description: "Compose deterministic local and model child Processes.",
		Version: "1.0.0", InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return agent.Deployment{}, err
	}
	definition := &compositionDefinition{descriptor: descriptor, local: local, model: model}
	return agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: rejectingDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("example-composition-implementation")),
		ConfigurationDigest: agent.ComputeDigest([]byte(
			"example-composition:" + local.Digest().String() + ":" + model.Digest().String(),
		)),
	})
}

func (definition *compositionDefinition) Descriptor() agent.Descriptor { return definition.descriptor }

func (definition *compositionDefinition) Start(input agent.Input) (agent.Execution, error) {
	decoded, err := agent.DecodeInput[compositionInput](input)
	if err != nil {
		return nil, err
	}
	return &compositionExecution{
		local: definition.local, model: definition.model,
		state: compositionState{Phase: "ready", Prompt: decoded.Prompt},
	}, nil
}

func (definition *compositionDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "example.composition" || state.SchemaVersion() != 1 {
		return nil, agent.ErrInvalidExecutionState
	}
	var decoded compositionState
	if err := json.Unmarshal(state.Payload(), &decoded); err != nil {
		return nil, err
	}
	return &compositionExecution{local: definition.local, model: definition.model, state: decoded}, nil
}

type compositionState struct {
	Phase    string   `json:"phase"`
	Prompt   string   `json:"prompt"`
	ChildIDs []string `json:"child_ids,omitempty"`
	WaitID   string   `json:"wait_id,omitempty"`
}

type compositionExecution struct {
	local agent.DeploymentRef
	model agent.DeploymentRef
	state compositionState
}

func (execution *compositionExecution) Step(
	_ context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	switch execution.state.Phase {
	case "ready":
		return execution.startChildren()
	case "children_started":
		return execution.waitForChildren(signals)
	case "wait_opened":
		if len(signals) == 0 {
			return agent.Transition{}, errors.New("composition wait was not opened")
		}
		opened, err := agent.ParseChildWaitOpened(signals[0])
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.WaitID = opened.WaitID().String()
		if len(signals) > 1 {
			return execution.complete(signals, uint32(len(signals)))
		}
		execution.state.Phase = "waiting"
		return agent.Wait(1, opened.WaitID())
	case "waiting":
		return execution.complete(signals, uint32(len(signals)))
	default:
		return agent.Transition{}, errors.New("composition execution cannot advance")
	}
}

func (execution *compositionExecution) startChildren() (agent.Transition, error) {
	localInput, _ := agent.EncodeInput(textInput{Text: execution.state.Prompt})
	modelInput, _ := agent.EncodeInput(interaction.Input{Messages: []chat.Message{
		chat.NewUserMessage(chat.NewTextPart(execution.state.Prompt)),
	}})
	localKey, _ := agent.ParseChildKey("local")
	modelKey, _ := agent.ParseChildKey("model")
	budget, _ := agent.NewBudget(20, 20, 40)
	localEffect, err := agent.StartChild(agent.ChildSpec{
		Key: localKey, Deployment: execution.local, Input: localInput, Budget: budget,
	})
	if err != nil {
		return agent.Transition{}, err
	}
	modelEffect, err := agent.StartChild(agent.ChildSpec{
		Key: modelKey, Deployment: execution.model, Input: modelInput, Budget: budget,
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = "children_started"
	return agent.Continue(0, localEffect, modelEffect)
}

func (execution *compositionExecution) waitForChildren(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 2 {
		return agent.Transition{}, errors.New("composition requires two child-start results")
	}
	for _, signal := range signals {
		started, err := agent.ParseChildStartResult(signal)
		if err != nil {
			return agent.Transition{}, err
		}
		if failure, failed := started.Failure(); failed {
			return agent.Fail(2, failure)
		}
		childID, _ := started.ProcessID()
		execution.state.ChildIDs = append(execution.state.ChildIDs, childID.String())
	}
	children := make([]agent.ProcessID, len(execution.state.ChildIDs))
	for index, encoded := range execution.state.ChildIDs {
		children[index], _ = agent.ParseProcessID(encoded)
	}
	waitKey, _ := agent.ParseWaitKey("composition")
	waitEffect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: waitKey, Children: children, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = "wait_opened"
	return agent.Continue(2, waitEffect)
}

func (execution *compositionExecution) complete(
	signals []agent.Signal,
	consumed uint32,
) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, errors.New("composition child results are missing")
	}
	completed, err := agent.ParseChildrenCompleted(signals[len(signals)-1])
	if err != nil {
		return agent.Transition{}, err
	}
	if completed.WaitID().String() != execution.state.WaitID {
		return agent.Transition{}, errors.New("composition received another wait's result")
	}
	var output compositionOutput
	for _, outcome := range completed.Outcomes() {
		result := outcome.Result()
		if result.Status() != agent.StatusCompleted {
			failure, _ := agent.NewFailure(
				agent.FailureKindExecution, "example.child.failed", "a composition child did not complete",
			)
			return agent.Fail(consumed, failure)
		}
		erased, _ := result.Output()
		switch outcome.Key().String() {
		case "local":
			decoded, err := agent.DecodeOutput[textOutput](erased)
			if err != nil {
				return agent.Transition{}, err
			}
			output.Local = decoded.Text
		case "model":
			decoded, err := agent.DecodeOutput[interaction.Output](erased)
			if err != nil {
				return agent.Transition{}, err
			}
			output.Model = decoded.ModelResponse.Text()
		default:
			return agent.Transition{}, errors.New("composition received an unknown child key")
		}
	}
	execution.state.Phase = "done"
	erased, _ := agent.EncodeOutput(output)
	return agent.Complete(consumed, erased)
}

func (execution *compositionExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("example.composition", 1, payload)
}

type compositionModel struct{}

func (compositionModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if len(request.Messages) == 0 {
		return nil, errors.New("composition model requires one message")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("model: " + request.Messages[len(request.Messages)-1].Text()))
	return &chat.Response{Choices: []chat.Choice{{
		Index: 0, Message: &message, FinishReason: "stop",
	}}}, nil
}

type rejectingDispatcher struct{}

func (rejectingDispatcher) Dispatch(
	context.Context,
	agent.EffectRequest,
	agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.Settlement{}, errors.New("definition emitted an unexpected dispatcher Effect")
}

func (rejectingDispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

type deploymentResolver map[agent.DeploymentRef]agent.Deployment

func (resolver deploymentResolver) Resolve(
	_ context.Context,
	reference agent.DeploymentRef,
) (agent.Deployment, error) {
	deployment, found := resolver[reference]
	if !found {
		return agent.Deployment{}, errors.New("exact Deployment is unavailable")
	}
	return deployment, nil
}

func decodeCompleted[T any](result agent.Result) (T, error) {
	var zero T
	erased, ok := result.Output()
	if !ok {
		return zero, fmt.Errorf("process ended with %s", result.Status())
	}
	return agent.DecodeOutput[T](erased)
}
