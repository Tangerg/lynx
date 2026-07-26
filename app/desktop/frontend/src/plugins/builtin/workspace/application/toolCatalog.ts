import type { Translate } from "@/lib/i18n";
import type { Tone } from "@/lib/tone";
import {
  useMCPServers,
  useMCPTools,
  type MCPServer,
} from "@/plugins/builtin/settings/mcp-servers/public/data";
import { toolCatalogGateway } from "./ports/toolCatalogGateway";
import { useWorkspaceBuiltinTools, type BuiltinToolInfo } from "./workspaceData";

export interface BuiltinToolSafetyPill {
  label: string;
  tone: Tone;
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
  mcpServers: MCPServer[];
  activeMcpServerCount: number;
  configuredMcpServerCount: number;
}

// A safety class reads as a tone; which fill and ink that tone wears is the
// Badge's business, not this layer's. These used to be Tailwind strings, tested
// as Tailwind strings — presentation decided one floor too low.
const TONE_BY_SAFETY: Record<string, Tone> = {
  safe: "accent",
  write: "warning",
  exec: "negative",
  network: "neutral",
};

export function useBuiltinToolConfigs() {
  return useWorkspaceBuiltinTools();
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

export function toolCatalogViewModel(servers: readonly MCPServer[]): ToolCatalogViewModel {
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
            tone: builtinToolSafetyTone(tool.safetyClass),
          }
        : undefined,
    })),
    isEmpty: tools.length === 0,
  };
}

export function toolCatalogSubtext(
  t: Translate,
  {
    activeMcpServerCount,
    configuredMcpServerCount,
  }: Pick<ToolCatalogViewModel, "activeMcpServerCount" | "configuredMcpServerCount">,
): string {
  return t("tools.mcpSubtext", {
    active: activeMcpServerCount,
    configured: configuredMcpServerCount,
  });
}

export function builtinToolSafetyTone(safetyClass: BuiltinToolInfo["safetyClass"]): Tone {
  if (!safetyClass) {
    return "neutral";
  }
  return TONE_BY_SAFETY[safetyClass] ?? "neutral";
}
