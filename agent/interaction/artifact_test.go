package interaction_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/tool"
)

func TestCompletionValidatorUsesOrderedTypedDelegateArtifacts(t *testing.T) {
	child := delegateWorkflow(t, "interaction.artifact_worker", func(input delegateRequest) (delegateResponse, error) {
		return delegateResponse{Value: "artifact:" + input.Value}, nil
	})
	budget, _ := agent.NewBudget(20, 20, 20)
	delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name: "delegate_artifact", Description: "Produce one typed artifact for completion validation.",
		Deployment: child, Budget: budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &artifactValidationModel{}
	validator := func(candidate interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
		workingContext := candidate.WorkingContext()
		if workingContext == nil || len(workingContext.Messages) == 0 {
			return interaction.CompletionDecision{}, errors.New("candidate has no WorkingContext")
		}
		workingContext.Messages[0] = chat.NewUserMessage(chat.NewTextPart("mutated"))
		if candidate.WorkingContext().Messages[0].Text() == "mutated" {
			return interaction.CompletionDecision{}, errors.New("WorkingContext aliases validator-owned copy")
		}
		artifacts := candidate.Artifacts().All()
		if len(artifacts) != 1 {
			return interaction.CompletionDecision{}, fmt.Errorf("artifact count = %d", len(artifacts))
		}
		artifact := artifacts[0]
		if artifact.DelegateName() != "delegate_artifact" {
			return interaction.CompletionDecision{}, fmt.Errorf("artifact Delegate identity is incorrect")
		}
		artifacts[0] = interaction.Artifact{}
		if candidate.Artifacts().All()[0].DelegateName() != "delegate_artifact" {
			return interaction.CompletionDecision{}, errors.New("Artifact snapshot aliases validator-owned slice")
		}
		decoded, err := artifact.Decode[delegateResponse]()
		if err != nil {
			return interaction.CompletionDecision{}, fmt.Errorf("decode artifact: %w", err)
		}
		if decoded.Value != "artifact:evidence" {
			return interaction.CompletionDecision{}, fmt.Errorf("decoded artifact = %#v", decoded)
		}
		if _, err := artifact.Decode[struct {
			Other string `json:"other"`
		}](); !errors.Is(err, interaction.ErrInvalidArtifact) || !errors.Is(err, agent.ErrInvalidOutput) {
			return interaction.CompletionDecision{}, fmt.Errorf("wrong typed decode error = %v", err)
		}
		output := candidate.Output()
		candidateText := ""
		if output.ModelResponse != nil {
			candidateText = output.ModelResponse.Text()
			output.ModelResponse.Output.Message = nil
			if candidate.Output().ModelResponse.Text() != candidateText {
				return interaction.CompletionDecision{}, errors.New("candidate Output aliases validator-owned copy")
			}
		}
		if candidateText == "premature" {
			return interaction.CompletionDecision{
				Feedback: "The delegated evidence is present, but the final answer must cite it explicitly.",
			}, nil
		}
		return interaction.CompletionDecision{Accepted: true}, nil
	}
	root := delegateInteractionWithValidator(
		t, model, nil, []interaction.Delegate{delegate}, validator, 4,
	)
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver: delegateResolver{child.DeploymentRef(): child},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), root, interactionInput(t, "produce validated evidence"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted || model.Calls() != 3 {
		t.Fatalf("status=%s model calls=%d", result.Status(), model.Calls())
	}
	erased, _ := result.Output()
	output, err := erased.Decode[interaction.Output]()
	if err != nil || output.ModelResponse == nil || output.ModelResponse.Text() != "artifact:evidence supports the answer" {
		t.Fatalf("output=%#v error=%v", output, err)
	}
}

func TestCompletionValidatorRetryHonorsModelCallLimit(t *testing.T) {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return textResponse("not accepted"), nil
	})
	root := delegateInteractionWithValidator(
		t, model, nil, nil,
		func(interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
			return interaction.CompletionDecision{Feedback: "Produce a verifiable final answer."}, nil
		},
		1,
	)
	engine, _ := agent.NewEngine(agent.EngineConfig{})
	result, err := engine.Run(context.Background(), root, interactionInput(t, "bounded validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	failure, present := result.Termination().Failure()
	if result.Status() != agent.StatusFailed || !present ||
		failure.Code() != "interaction.limit.model_calls" || failure.Kind() != agent.FailureKindExecution {
		t.Fatalf("termination=%#v", result.Termination())
	}
}

func TestCompletionDecisionContract(t *testing.T) {
	cases := []struct {
		name     string
		decision interaction.CompletionDecision
		valid    bool
	}{
		{name: "accepted", decision: interaction.CompletionDecision{Accepted: true}, valid: true},
		{name: "retry", decision: interaction.CompletionDecision{Feedback: "Add evidence."}, valid: true},
		{name: "zero", decision: interaction.CompletionDecision{}},
		{name: "accepted with feedback", decision: interaction.CompletionDecision{Accepted: true, Feedback: "unused"}},
		{name: "untrimmed feedback", decision: interaction.CompletionDecision{Feedback: " retry"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := test.decision.Valid(); got != test.valid {
				t.Fatalf("Valid()=%t, want %t", got, test.valid)
			}
		})
	}
	if _, err := (interaction.Artifact{}).Decode[delegateResponse](); !errors.Is(err, interaction.ErrInvalidArtifact) {
		t.Fatalf("DecodeArtifact(zero) error=%v", err)
	}
}

func TestCompletionValidatorCanRejectDirectToolResult(t *testing.T) {
	type echoInput struct {
		Value string `json:"value"`
	}
	echo, err := tool.NewFunc(tool.FuncConfig{
		Name: "direct_echo", Description: "Return text as a direct semantic result.",
	}, func(_ context.Context, input echoInput) (string, error) { return input.Value, nil })
	if err != nil {
		t.Fatal(err)
	}
	model := &directCompletionValidationModel{}
	validator := func(candidate interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
		if candidate.Artifacts().Len() != 0 {
			return interaction.CompletionDecision{}, errors.New("ordinary Tool produced a Delegate Artifact")
		}
		if candidate.Output().Source == interaction.CompletionSourceDirectToolResults {
			return interaction.CompletionDecision{Feedback: "Explain the direct result before completing."}, nil
		}
		return interaction.CompletionDecision{Accepted: true}, nil
	}
	root := delegateInteractionWithValidator(
		t, model, []tool.Tool{directTool{Tool: echo}}, nil, validator, 3,
	)
	engine, _ := agent.NewEngine(agent.EngineConfig{})
	result, err := engine.Run(context.Background(), root, interactionInput(t, "echo and explain"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted || model.Calls() != 2 {
		t.Fatalf("status=%s calls=%d", result.Status(), model.Calls())
	}
}

func TestCompletionValidatorRejectsInvalidDecision(t *testing.T) {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return textResponse("candidate"), nil
	})
	root := delegateInteractionWithValidator(
		t, model, nil, nil,
		func(interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
			return interaction.CompletionDecision{}, nil
		},
		2,
	)
	engine, _ := agent.NewEngine(agent.EngineConfig{})
	result, err := engine.Run(context.Background(), root, interactionInput(t, "invalid validator"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	failure, present := result.Termination().Failure()
	if result.Status() != agent.StatusFailed || result.Termination().Cause() != agent.TerminationCauseContractFailure ||
		!present || failure.Kind() != agent.FailureKindContract ||
		failure.Code() != "interaction.completion.decision_invalid" {
		t.Fatalf("termination=%#v", result.Termination())
	}
}

func TestCompletionValidatorFailureClassification(t *testing.T) {
	cases := []struct {
		name      string
		validator interaction.CompletionValidator
		cause     agent.TerminationCause
		kind      agent.FailureKind
		code      string
	}{
		{
			name: "returned error",
			validator: func(interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
				return interaction.CompletionDecision{}, errors.New("validator cannot decide")
			},
			cause: agent.TerminationCauseExecutionFailure,
			kind:  agent.FailureKindExecution,
			code:  "interaction.completion.validator_failed",
		},
		{
			name: "panic",
			validator: func(interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
				panic("validator panic")
			},
			cause: agent.TerminationCausePanic,
			kind:  agent.FailureKindPanic,
			code:  "execution.step.failed",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return textResponse("candidate"), nil
			})
			root := delegateInteractionWithValidator(t, model, nil, nil, test.validator, 2)
			engine, _ := agent.NewEngine(agent.EngineConfig{})
			result, err := engine.Run(context.Background(), root, interactionInput(t, "validator failure"))
			if err != nil {
				t.Fatal(err)
			}
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			failure, present := result.Termination().Failure()
			if result.Termination().Cause() != test.cause || !present ||
				failure.Kind() != test.kind || failure.Code() != test.code {
				t.Fatalf("termination=%#v", result.Termination())
			}
		})
	}
}

type artifactValidationModel struct {
	mu    sync.Mutex
	calls int
}

func (a *artifactValidationModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	switch a.calls {
	case 1:
		return toolCallResponse(chat.ToolCall{
			ID: "call_artifact", Name: "delegate_artifact", Arguments: `{"value":"evidence"}`,
		}), nil
	case 2:
		return textResponse("premature"), nil
	case 3:
		if len(request.Messages) != 5 || request.Messages[3].Role != chat.RoleAssistant ||
			request.Messages[3].Text() != "premature" || request.Messages[4].Role != chat.RoleUser ||
			request.Messages[4].Text() != "The delegated evidence is present, but the final answer must cite it explicitly." {
			return nil, fmt.Errorf("completion feedback context = %#v", request.Messages)
		}
		return textResponse("artifact:evidence supports the answer"), nil
	default:
		return nil, errors.New("unexpected model call")
	}
}

func (a *artifactValidationModel) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

type directCompletionValidationModel struct {
	mu    sync.Mutex
	calls int
}

func (d *directCompletionValidationModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.calls == 1 {
		return toolCallResponse(chat.ToolCall{
			ID: "call_direct_validation", Name: "direct_echo", Arguments: `{"value":"direct"}`,
		}), nil
	}
	if len(request.Messages) != 4 || request.Messages[1].Role != chat.RoleAssistant ||
		request.Messages[2].Role != chat.RoleTool || request.Messages[3].Role != chat.RoleUser ||
		request.Messages[3].Text() != "Explain the direct result before completing." {
		return nil, fmt.Errorf("direct completion feedback context = %#v", request.Messages)
	}
	return textResponse("direct, explained"), nil
}

func (d *directCompletionValidationModel) Calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}
