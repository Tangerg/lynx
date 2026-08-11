// Package backend defines the application boundary assembled by a runtime
// adapter. Each use case still consumes its own narrow port; Services only
// gives the process composition root one explicit, discoverable manifest.
package backend

import (
	"errors"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

// Services is one coherent connection to a backend runtime.
type Services struct {
	Agent      agent.Runtime
	Workspaces workspace.Service
	Changes    changefeed.Source
	Transfers  sessiontransfer.Service
}

// AgentOnly builds the intentionally reduced composition used by the scripted
// demo runtime and focused tests. Auxiliary commands stay visible but explain
// that their service is unavailable.
func AgentOnly(runtime agent.Runtime) Services {
	return Services{Agent: runtime}
}

// Validate checks the minimum contract every CLI mode requires. Auxiliary
// services are optional because a negotiated runtime composition may omit them.
func (services Services) Validate() error {
	if services.Agent == nil {
		return errors.New("backend services: agent runtime is required")
	}
	return nil
}
