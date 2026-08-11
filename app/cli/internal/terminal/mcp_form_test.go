package terminal

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"

	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

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
