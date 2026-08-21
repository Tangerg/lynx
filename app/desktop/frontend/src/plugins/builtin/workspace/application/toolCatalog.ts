import type { Translate } from "@/lib/i18n";
import type { Tone } from "@/lib/tone";
import { TOOL_FAMILIES, TOOL_ICON_BY_NAME, toolFamilyId } from "@/lib/toolFamilies";
import {
  useMCPServers,
  useMCPTools,
  type MCPServerSettings,
} from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { useWorkspaceBuiltinTools, type BuiltinToolSummary } from "./workspaceQueries";

export interface BuiltinToolSafetyPill {
  label: string;
  tone: Tone;
}

export interface BuiltinToolRowViewModel {
  id: string;
  name: string;
  description: string;
  icon: string;
  parameters: Record<string, unknown>;
  safety?: BuiltinToolSafetyPill;
}

/** One family of the catalog, named by its i18n key so the ring stays wordless. */
export interface BuiltinToolFamilyViewModel {
  id: string;
  titleKey: string;
  rows: BuiltinToolRowViewModel[];
}

export interface BuiltinToolCatalogViewModel {
  families: BuiltinToolFamilyViewModel[];
  isEmpty: boolean;
}

export interface ToolCatalogViewModel {
  mcpServers: MCPServerSettings[];
  activeMcpServerCount: number;
  configuredMcpServerCount: number;
}

// A safety class reads as a tone; which fill and ink that tone wears is the
// Badge's business, not this layer's. Application exposes semantic tone only.
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

export function toolCatalogViewModel(servers: readonly MCPServerSettings[]): ToolCatalogViewModel {
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

/**
 * The runtime's tools, grouped into the families someone browsing them asks in.
 *
 * Driven by `tools.list` and only ANNOTATED by the local table: what exists is the
 * runtime's answer, so a tool it stops shipping disappears from here without an
 * edit, and one it adds before the client knows the name still lists — under the
 * trailing family, with the generic glyph. A catalog that enumerated the table
 * instead would advertise whatever the client believed rather than what it can call.
 *
 * Family order is the table's, never by size: the reader is scanning for a heading,
 * and a catalog that reshuffles itself as a runtime gains a tool is one where
 * nothing is ever in the place it was last time.
 */
export function builtinToolCatalogViewModel(
  tools: readonly BuiltinToolSummary[],
): BuiltinToolCatalogViewModel {
  const byFamily = new Map<string, BuiltinToolRowViewModel[]>();
  for (const tool of tools) {
    const family = toolFamilyId(tool.name) ?? UNPLACED_FAMILY;
    const rows = byFamily.get(family) ?? [];
    rows.push({
      id: tool.name,
      name: tool.name,
      description: tool.description,
      icon: TOOL_ICON_BY_NAME[tool.name] ?? GENERIC_TOOL_ICON,
      parameters: tool.parameters,
      safety: tool.safetyClass
        ? {
            label: tool.safetyClass,
            tone: builtinToolSafetyTone(tool.safetyClass),
          }
        : undefined,
    });
    byFamily.set(family, rows);
  }

  const order = [...TOOL_FAMILIES.map((family) => family.id), UNPLACED_FAMILY];
  return {
    families: order.flatMap((id) => {
      const rows = byFamily.get(id);
      return rows ? [{ id, titleKey: `tools.family.${id}`, rows }] : [];
    }),
    isEmpty: tools.length === 0,
  };
}

const UNPLACED_FAMILY = "other";
const GENERIC_TOOL_ICON = "tool";

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

export function builtinToolSafetyTone(safetyClass: BuiltinToolSummary["safetyClass"]): Tone {
  if (!safetyClass) {
    return "neutral";
  }
  return TONE_BY_SAFETY[safetyClass] ?? "neutral";
}
