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

func (m *mcpFormFlow) replacesConnection() bool {
	return m.mode != mcpFormUpdate || m.draft.connectionMode == "replace"
}

func (m *mcpFormFlow) connectionStep() mcpFormStep {
	if m.draft.transport == string(mcp.Stdio) {
		return mcpFormStdio
	}
	return mcpFormHTTP
}

func (m *mcpFormFlow) advance() bool {
	switch m.step {
	case mcpFormGeneral:
		if m.replacesConnection() {
			m.step = m.connectionStep()
		} else {
			m.step = mcpFormPolicy
		}
		return true
	case mcpFormHTTP, mcpFormStdio:
		m.step = mcpFormPolicy
		return true
	default:
		return false
	}
}

func (m *mcpFormFlow) back() bool {
	switch m.step {
	case mcpFormHTTP, mcpFormStdio:
		m.step = mcpFormGeneral
		return true
	case mcpFormPolicy:
		if m.replacesConnection() {
			m.step = m.connectionStep()
		} else {
			m.step = mcpFormGeneral
		}
		return true
	default:
		return false
	}
}

func (m *mcpFormFlow) progress() (int, int, string) {
	total := 2
	if m.replacesConnection() {
		total = 3
	}
	switch m.step {
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

func (m *mcpFormFlow) clearSecrets() {
	m.draft.authorization, m.draft.headers, m.draft.environment = "", "", ""
	for _, field := range m.secretFields {
		field.Editor().SetText("")
	}
	clear(m.secretFields)
	m.secretFields = nil
}
