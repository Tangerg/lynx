import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";

export type MCPTransport = "stdio" | "streamableHttp";
export type MCPServerStatus =
  "disabled" | "disconnected" | "connecting" | "connected" | "failed" | "needsAuth";

// MCPServerSettings is the frontend's unified MCP resource. Workspace and
// settings views intentionally consume the same read model instead of joining
// separate configuration and live-status caches.
export interface MCPServerSettings {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: MCPServerStatus;
  errorDetail?: string;
  icon: string;
  type: MCPTransport;
  enabled: boolean;
  description?: string;
  url?: string;
  authorizationMasked?: string;
  headersMasked?: Record<string, string>;
  command?: string;
  args?: string[];
  envMasked?: Record<string, string>;
  dir?: string;
  timeoutSeconds?: number;
  disabledTools?: string[];
  autoApproveTools?: string[];
  toolCount?: number;
}

export interface MCPToolSummary {
  name: string;
  description: string;
}

export interface McpToolsQuery {
  server: string;
}

export const MCP_SERVERS_KEY = "mcp-servers";
export const MCP_TOOLS_KEY = "mcp-tools";

const MCP_ICON: Record<string, string> = {
  Filesystem: "folder",
  Git: "branch",
  Shell: "terminal",
  "Web Search": "globe",
  Linear: "list",
  GitHub: "git",
  Postgres: "tool",
  Slack: "chat",
};

export function mcpServerIcon(name: string): string {
  return MCP_ICON[name] ?? "tool";
}

export const useMCPServers = createDataQuery<MCPServerSettings[]>(MCP_SERVERS_KEY);
export const useMCPTools = createParameterizedDataQuery<McpToolsQuery, MCPToolSummary[]>(
  MCP_TOOLS_KEY,
);
