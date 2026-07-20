package event

import (
	"encoding/json"
	"errors"
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
	var bindings core.Bindings
	bindings.Set("input", func() {})
	raw, err := json.Marshal(ProcessCreated{
		Header:   NewHeader("proc"),
		Bindings: bindings,
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

func TestProcessSnapshotFailedMarshalUsesEventEnvelope(t *testing.T) {
	raw, err := json.Marshal(ProcessSnapshotFailed{
		Header: NewHeader("proc"),
		Policy: "report_only",
		Err:    errors.New("store unavailable"),
	})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var got struct {
		Kind      string `json:"kind"`
		ProcessID string `json:"process_id"`
		Payload   struct {
			Policy string `json:"policy"`
			Error  string `json:"error"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Kind != "process_snapshot_failed" || got.ProcessID != "proc" {
		t.Fatalf("envelope = kind %q process %q", got.Kind, got.ProcessID)
	}
	if got.Payload.Policy != "report_only" || got.Payload.Error != "store unavailable" {
		t.Fatalf("payload = %+v", got.Payload)
	}
}
