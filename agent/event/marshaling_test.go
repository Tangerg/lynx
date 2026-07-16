package event

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestAgentDeployedMarshalCarriesExactDeployment(t *testing.T) {
	want := core.DeploymentRef{Name: "writer", Version: "1.2.3", Digest: "digest"}
	raw, err := json.Marshal(AgentDeployed{Header: NewHeader(""), Deployment: want})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Payload struct {
			Deployment core.DeploymentRef `json:"deployment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Payload.Deployment != want {
		t.Fatalf("deployment = %#v, want %#v", got.Payload.Deployment, want)
	}
}

func TestProcessCompletedMarshal_SummarizesOpaqueResult(t *testing.T) {
	raw, err := json.Marshal(ProcessCompleted{
		Header: NewHeader("proc"),
		Result: func() {},
	})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var got struct {
		Payload struct {
			Result string `json:"result"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Payload.Result == "" {
		t.Fatalf("result = %q, want fallback string", got.Payload.Result)
	}
}

func TestProcessCreatedMarshal_SummarizesOpaqueBindings(t *testing.T) {
	raw, err := json.Marshal(ProcessCreated{
		Header:   NewHeader("proc"),
		Bindings: map[string]any{"input": func() {}},
	})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var got struct {
		Payload struct {
			Bindings map[string]string `json:"bindings"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Payload.Bindings["input"] == "" {
		t.Fatalf("bindings = %+v, want fallback string", got.Payload.Bindings)
	}
}
