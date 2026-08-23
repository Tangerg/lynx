// Package mcpserver owns durable MCP configuration independent of transport
// sessions and wire presentation.
package mcpserver

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

var (
	ErrNotFound = errors.New("mcpserver: not found")
	ErrExists = errors.New("mcpserver: already exists")
)

type Record struct {
	Name             string                `json:"name"`
	Enabled          bool                  `json:"enabled"`
	Description      string                `json:"description,omitempty"`
	Transport        protocol.MCPTransport `json:"transport"`
	URL              string                `json:"url,omitempty"`
	Command          string                `json:"command,omitempty"`
	Args             []string              `json:"args,omitempty"`
	Dir              string                `json:"dir,omitempty"`
	TimeoutSeconds   int                   `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string              `json:"disabledTools,omitempty"`
	AutoApproveTools []string              `json:"autoApproveTools,omitempty"`
	AuthorizationSet bool                  `json:"authorizationSet,omitempty"`
	OAuthSet         bool                  `json:"oauthSet,omitempty"`
	HeaderNames      []string              `json:"headerNames,omitempty"`
	EnvironmentNames []string              `json:"environmentNames,omitempty"`
	UpdatedAt        time.Time             `json:"updatedAt"`
}

type Secrets struct {
	Authorization string            `json:"authorization,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	OAuthOrigin   string            `json:"oauthOrigin,omitempty"`
	OAuthSession  []byte            `json:"oauthSession,omitempty"`
}
