// Package backend defines the application boundary assembled by a runtime
// adapter. Each use case still consumes its own narrow port; Services only
// gives the process composition root one explicit, discoverable manifest.
package backend

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
	"github.com/Tangerg/lynx/app/cli/internal/feedback"
	"github.com/Tangerg/lynx/app/cli/internal/goal"
	"github.com/Tangerg/lynx/app/cli/internal/hookpolicy"
	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
	"github.com/Tangerg/lynx/app/cli/internal/mcp"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/schedule"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/skills"
	"github.com/Tangerg/lynx/app/cli/internal/usage"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

// Services is one coherent connection to a backend runtime.
type Services struct {
	Agent            agent.Runtime
	RuntimeProfile   *runtimeprofile.Profile
	Workspaces       workspace.Service
	Changes          changefeed.Source
	Transfers        sessiontransfer.Service
	Usage            usage.Service
	ModelConfig      modelconfig.Service
	Goals            goal.Service
	Skills           skills.Service
	MCP              mcp.Service
	Schedules        schedule.Service
	AgentMemory      agentmemory.Service
	Knowledge        knowledge.Service
	DiagnosticTools  diagnostictool.Service
	AuthoringContext authoringcontext.Service
	Hooks            hookpolicy.Service
	Feedback         feedback.Service
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
	if services.RuntimeProfile != nil {
		if err := services.RuntimeProfile.Validate(); err != nil {
			return fmt.Errorf("backend services: %w", err)
		}
	}
	return nil
}
