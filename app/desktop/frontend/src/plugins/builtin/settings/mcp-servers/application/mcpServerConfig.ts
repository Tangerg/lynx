import { useCallback } from "react";
import {
  MCP_CONFIGS_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  type MCPServerConfigInfo,
  useMCPConfigs,
} from "./mcpServerQueries";
import { queryClient } from "@/lib/data/queryClient";
import type { MCPServerConfigInput } from "./mcpServerInput";
import { mcpServerGateway, type MCPServerTestOutcome } from "./ports/mcpServerGateway";
export type { MCPServerConfigInput, MCPServerTransport } from "./mcpServerInput";
export type { MCPServerTestOutcome } from "./ports/mcpServerGateway";

// MCP server-configuration mutations. Counterpart to the read-side
// useMCPConfigs() query: mutators invalidate the configs + status views so the
// pane and the Tools workspace view both re-read the new registry state.

export type MCPServerConfig = MCPServerConfigInfo;

export function useMCPServerConfigs() {
  return useMCPConfigs();
}

// Every mutator refetches: the editable registry (the pane), the status-only
// sidebar/Tools view, and the per-server tool detail (tool gating + reconnect
// hot-swap the visible set).
function invalidateMcp(): Promise<void> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: [MCP_CONFIGS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [MCP_SERVERS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [MCP_TOOLS_KEY] }),
  ]).then(() => undefined);
}

/**
 * Upsert an MCP server config and refetch the MCP views. `authorization` is the
 * raw token; omitting it keeps the stored one, so editing a non-secret field
 * never re-prompts for the token.
 */
export function useConfigureMCPServer(): (input: MCPServerConfigInput) => Promise<void> {
  return useCallback(async (input) => {
    await mcpServerGateway().configure(input);
    await invalidateMcp();
  }, []);
}

/** Remove a server from the registry. */
export function useRemoveMCPServer(): (name: string) => Promise<void> {
  return useCallback(async (name) => {
    await mcpServerGateway().remove(name);
    await invalidateMcp();
  }, []);
}

/** Flip a server's enablement without re-sending its whole config. */
export function useSetMCPServerEnabled(): (name: string, enabled: boolean) => Promise<void> {
  return useCallback(async (name, enabled) => {
    await mcpServerGateway().setEnabled(name, enabled);
    await invalidateMcp();
  }, []);
}

/**
 * Start the interactive OAuth sign-in for a server. The connection outcome
 * arrives via mcp.serverChanged (the events plugin re-invalidates the MCP
 * views), so this just kicks it off.
 */
export function useAuthorizeMCPServer(): (name: string) => Promise<void> {
  return useCallback(async (name) => {
    await mcpServerGateway().authorize(name);
    await invalidateMcp();
  }, []);
}

/**
 * Live-probe a server config without persisting it. A failed probe comes back
 * as `{ ok:false, error }`, so callers render the reason inline.
 */
export function useTestMCPServer(): (input: MCPServerConfigInput) => Promise<MCPServerTestOutcome> {
  return useCallback((input) => mcpServerGateway().test(input), []);
}
