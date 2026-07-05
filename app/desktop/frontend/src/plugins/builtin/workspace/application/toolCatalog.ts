import type { BuiltinToolInfo, MCPServer } from "@/lib/data/queries";
import { useBuiltinTools, useMCPServers, useMCPTools } from "@/lib/data/queries";
import { toolCatalogGateway } from "./ports/toolCatalogGateway";

export type MCPServerConfig = MCPServer;

export interface BuiltinToolSafetyPill {
  label: string;
  className: string;
}

export interface BuiltinToolRowViewModel {
  id: string;
  name: string;
  description: string;
  safety?: BuiltinToolSafetyPill;
}

export interface BuiltinToolCatalogViewModel {
  rows: BuiltinToolRowViewModel[];
  isEmpty: boolean;
}

export interface ToolCatalogViewModel {
  mcpServers: MCPServerConfig[];
  activeMcpServerCount: number;
  configuredMcpServerCount: number;
}

const SAFETY_PILL_CLASS_BY_SAFETY: Record<string, string> = {
  safe: "bg-accent/12 text-accent",
  write: "bg-warning/12 text-warning",
  exec: "bg-negative/12 text-negative",
  network: "bg-surface-2 text-fg-muted",
};

const DEFAULT_SAFETY_PILL_CLASS = "bg-surface-2 text-fg-muted";

export function useBuiltinToolConfigs() {
  return useBuiltinTools();
}

export function useMCPServerConfigs() {
  return useMCPServers();
}

export function useMCPServerToolConfigs(server: string) {
  return useMCPTools({ server });
}

export function reconnectMCPServer(server: string): void {
  toolCatalogGateway()
    .reconnectMCPServer(server)
    .catch((err: unknown) => console.warn("[mcp] reconnect failed:", err));
}

export function toolCatalogViewModel(servers: readonly MCPServerConfig[]): ToolCatalogViewModel {
  let activeMcpServerCount = 0;
  for (const server of servers) {
    if (server.status === "connected") {
      activeMcpServerCount += 1;
    }
  }

  return {
    mcpServers: Array.from(servers),
    activeMcpServerCount,
    configuredMcpServerCount: servers.length,
  };
}

export function builtinToolCatalogViewModel(
  tools: readonly BuiltinToolInfo[],
): BuiltinToolCatalogViewModel {
  return {
    rows: tools.map((tool) => ({
      id: tool.name,
      name: tool.name,
      description: tool.description,
      safety: tool.safetyClass
        ? {
            label: tool.safetyClass,
            className: builtinToolSafetyPillClassName(tool.safetyClass),
          }
        : undefined,
    })),
    isEmpty: tools.length === 0,
  };
}

export function toolCatalogSubtext({
  activeMcpServerCount,
  configuredMcpServerCount,
}: Pick<ToolCatalogViewModel, "activeMcpServerCount" | "configuredMcpServerCount">): string {
  return `${activeMcpServerCount} MCP active · ${configuredMcpServerCount} configured`;
}

export function builtinToolSafetyPillClassName(
  safetyClass: BuiltinToolInfo["safetyClass"],
): string {
  if (!safetyClass) {
    return DEFAULT_SAFETY_PILL_CLASS;
  }
  return SAFETY_PILL_CLASS_BY_SAFETY[safetyClass] ?? DEFAULT_SAFETY_PILL_CLASS;
}
