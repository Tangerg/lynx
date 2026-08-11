package terminal

import (
	"github.com/Tangerg/oolong/components/headless"

	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

type mcpFormStep uint8

const (
	mcpFormGeneral mcpFormStep = iota + 1
	mcpFormHTTP
	mcpFormStdio
	mcpFormPolicy
)

type mcpFormFlow struct {
	mode         mcpFormMode
	server       mcp.Server
	draft        mcpFormDraft
	step         mcpFormStep
	secretFields []*headless.Text
}

func newMCPFormFlow(mode mcpFormMode, server mcp.Server) *mcpFormFlow {
	return &mcpFormFlow{
		mode: mode, server: server.Clone(), draft: newMCPFormDraft(mode, server), step: mcpFormGeneral,
	}
}

func (f *mcpFormFlow) replacesConnection() bool {
	return f.mode != mcpFormUpdate || f.draft.connectionMode == "replace"
}

func (f *mcpFormFlow) connectionStep() mcpFormStep {
	if f.draft.transport == string(mcp.Stdio) {
		return mcpFormStdio
	}
	return mcpFormHTTP
}

func (f *mcpFormFlow) advance() bool {
	switch f.step {
	case mcpFormGeneral:
		if f.replacesConnection() {
			f.step = f.connectionStep()
		} else {
			f.step = mcpFormPolicy
		}
		return true
	case mcpFormHTTP, mcpFormStdio:
		f.step = mcpFormPolicy
		return true
	default:
		return false
	}
}

func (f *mcpFormFlow) back() bool {
	switch f.step {
	case mcpFormHTTP, mcpFormStdio:
		f.step = mcpFormGeneral
		return true
	case mcpFormPolicy:
		if f.replacesConnection() {
			f.step = f.connectionStep()
		} else {
			f.step = mcpFormGeneral
		}
		return true
	default:
		return false
	}
}

func (f *mcpFormFlow) progress() (int, int, string) {
	total := 2
	if f.replacesConnection() {
		total = 3
	}
	switch f.step {
	case mcpFormGeneral:
		return 1, total, "General"
	case mcpFormHTTP:
		return 2, total, "HTTP connection"
	case mcpFormStdio:
		return 2, total, "stdio connection"
	default:
		return total, total, "Tool policy"
	}
}

func (f *mcpFormFlow) clearSecrets() {
	f.draft.authorization, f.draft.headers, f.draft.environment = "", "", ""
	for _, field := range f.secretFields {
		field.Editor().SetText("")
	}
	clear(f.secretFields)
	f.secretFields = nil
}
