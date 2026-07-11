import { createDataQuery, createParameterizedDataQuery } from "@/lib/data/dataQuery";

export interface MCPServer {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";
  errorDetail?: string;
  icon: string;
}

export interface McpToolInfo {
  name: string;
  description: string;
}

export interface McpToolsQuery {
  server: string;
}

export type MCPTransport = "stdio" | "streamableHttp";

export interface MCPServerConfigInfo {
  name: string;
  type: MCPTransport;
  enabled: boolean;
  description?: string;
  url?: string;
  authorizationMasked?: string;
  headers?: Record<string, string>;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  dir?: string;
  timeoutSeconds?: number;
  disabledTools?: string[];
  autoApproveTools?: string[];
  status?: MCPServer["status"];
  toolCount?: number;
  errorDetail?: string;
}

export const MCP_SERVERS_KEY = "mcp-servers";
export const MCP_CONFIGS_KEY = "mcp-configs";
export const MCP_TOOLS_KEY = "mcp-tools";

export const useMCPServers = createDataQuery<MCPServer[]>(MCP_SERVERS_KEY);
export const useMCPConfigs = createDataQuery<MCPServerConfigInfo[]>(MCP_CONFIGS_KEY);
export const useMCPTools = createParameterizedDataQuery<McpToolsQuery, McpToolInfo[]>(
  MCP_TOOLS_KEY,
);
