package config

import "os"

type environmentVariable string

const (
	environmentPrefix environmentVariable = "SCOPEAPP"

	mcpServersEnvironment   environmentVariable = "SCOPEAPP_MCP_SERVERS"
	a2aAgentsEnvironment    environmentVariable = "SCOPEAPP_A2A_AGENTS"
	a2aOriginsEnvironment   environmentVariable = "SCOPEAPP_A2A_RPC_ORIGINS"
	jinaAPIKeyEnvironment   environmentVariable = "SCOPEAPP_JINA_API_KEY"
	tavilyAPIKeyEnvironment environmentVariable = "SCOPEAPP_TAVILY_API_KEY"
	httpHostsEnvironment    environmentVariable = "SCOPEAPP_HTTP_ALLOWED_HOSTS"

	mcpTokenEnvironmentPrefix = "SCOPEAPP_MCP_"
	mcpTokenEnvironmentSuffix = "_TOKEN"
)

func (e environmentVariable) String() string { return string(e) }

func (e environmentVariable) Value() string { return os.Getenv(e.String()) }

func mcpTokenEnvironment(name string) environmentVariable {
	return environmentVariable(mcpTokenEnvironmentPrefix + envTokenKey(name) + mcpTokenEnvironmentSuffix)
}
