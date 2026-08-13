package terminal

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"

	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

func TestMCPFormRejectsSubmissionFromAReplacedStepPresentation(t *testing.T) {
	transcript := testTranscriptView(t)
	application := &app{loop: &program.Runtime{}, transcript: transcript}
	application.stack.SetBase(transcript)
	flow := newMCPFormFlow(mcpFormCreate, mcp.Server{})
	flow.draft.name = "docs"
	flow.draft.transport = string(mcp.StreamableHTTP)
	application.showMCPFormStep(flow)
	drawRoot(t, &application.stack, 96, 28)

	oldDialog := application.mcpDialog
	application.showMCPFormStep(flow)
	application.stack.Handle(input.Key{Code: input.Enter})
	if flow.step != mcpFormGeneral {
		t.Fatalf("stale form advanced to step %d", flow.step)
	}
	if application.mcpDialog == oldDialog || !application.mcpDialog.Open() {
		t.Fatal("stale form replaced or dismissed the current dialog")
	}

	drawRoot(t, &application.stack, 96, 28)
	application.stack.Handle(input.Key{Code: input.Enter})
	if flow.step != mcpFormHTTP {
		t.Fatalf("current form advanced to step %d, want HTTP", flow.step)
	}
	application.closeMCPForm(flow)
}

func TestMCPFormFlowRoutesOnlyThroughRelevantConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		mode           mcpFormMode
		transport      mcp.Transport
		connectionMode string
		want           []mcpFormStep
	}{
		{name: "create HTTP", mode: mcpFormCreate, transport: mcp.StreamableHTTP, want: []mcpFormStep{mcpFormGeneral, mcpFormHTTP, mcpFormPolicy}},
		{name: "create stdio", mode: mcpFormCreate, transport: mcp.Stdio, want: []mcpFormStep{mcpFormGeneral, mcpFormStdio, mcpFormPolicy}},
		{name: "update keeping connection", mode: mcpFormUpdate, transport: mcp.StreamableHTTP, connectionMode: "keep", want: []mcpFormStep{mcpFormGeneral, mcpFormPolicy}},
		{name: "update replacing connection", mode: mcpFormUpdate, transport: mcp.Stdio, connectionMode: "replace", want: []mcpFormStep{mcpFormGeneral, mcpFormStdio, mcpFormPolicy}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			flow := newMCPFormFlow(test.mode, mcp.Server{})
			flow.draft.transport = string(test.transport)
			if test.connectionMode != "" {
				flow.draft.connectionMode = test.connectionMode
			}
			visited := []mcpFormStep{flow.step}
			for flow.advance() {
				visited = append(visited, flow.step)
			}
			if !slices.Equal(visited, test.want) {
				t.Fatalf("forward steps = %v, want %v", visited, test.want)
			}
			for index := len(test.want) - 2; index >= 0; index-- {
				if !flow.back() || flow.step != test.want[index] {
					t.Fatalf("back step = %v, want %v", flow.step, test.want[index])
				}
			}
			if flow.back() {
				t.Fatal("flow moved before the general step")
			}
		})
	}
}

func TestMCPFormFlowClearsEverySecretProjection(t *testing.T) {
	t.Parallel()
	flow := newMCPFormFlow(mcpFormCreate, mcp.Server{})
	flow.draft.authorization = "Bearer private"
	flow.draft.headers = `{"X-Key":"private"}`
	flow.draft.environment = `{"TOKEN":"private"}`
	for range 3 {
		field := &headless.Text{}
		field.Editor().SetText("private")
		flow.secretFields = append(flow.secretFields, field)
	}
	fields := slices.Clone(flow.secretFields)

	flow.clearSecrets()
	if flow.draft.authorization != "" || flow.draft.headers != "" || flow.draft.environment != "" || flow.secretFields != nil {
		t.Fatalf("secret state survived cleanup: %+v", flow)
	}
	for index, field := range fields {
		if value := field.Editor().Text(); value != "" {
			t.Fatalf("secret field %d = %q after cleanup", index, value)
		}
	}
}

func TestMCPToolNameValidationRejectsDuplicates(t *testing.T) {
	t.Parallel()
	if err := validateMCPToolNames("read, write, read"); err == nil {
		t.Fatal("duplicate tool names were accepted")
	}
	if err := validateMCPToolNames("read, write"); err != nil {
		t.Fatalf("unique tool names were rejected: %v", err)
	}
}
