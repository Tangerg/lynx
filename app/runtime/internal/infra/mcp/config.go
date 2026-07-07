package mcp

import lynxmcp "github.com/Tangerg/lynx/mcp"

// ServerConfig is the MCP server descriptor, re-exported so callers configure
// connections without importing the lynx mcp module directly.
type ServerConfig = lynxmcp.ServerConfig

// Transport and its values are re-exported alongside ServerConfig so the
// composition layer builds configs without importing the lynx mcp module.
type Transport = lynxmcp.Transport

const (
	TransportHTTP  = lynxmcp.TransportHTTP
	TransportStdio = lynxmcp.TransportStdio
)
