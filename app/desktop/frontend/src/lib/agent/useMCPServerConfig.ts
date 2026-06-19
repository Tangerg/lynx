import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { t } from "@/lib/i18n";
import { errorDetail, type ConfigureMCPServerRequest } from "@/rpc";
import { MCP_CONFIGS_KEY, MCP_SERVERS_KEY, MCP_TOOLS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";

// MCP server-configuration mutations (workspace.mcp.configure / remove /
// setEnabled / test). Lives in lib/ so the settings pane (a component) reaches
// the runtime through a hook rather than importing @/rpc / @/main directly
// (layer rule). Counterpart to the read-side useMCPConfigs() query — the
// mutators invalidate the configs + status views so the pane and the Tools
// workspace view both re-read the new registry state.

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
 * Upsert an MCP server config (workspace.mcp.configure) and refetch the MCP
 * views. `authorization` is the RAW token; omitting it keeps the stored one,
 * so editing a non-secret field never re-prompts for the token.
 */
export function useConfigureMCPServer(): (params: ConfigureMCPServerRequest) => Promise<void> {
  return useCallback(async (params) => {
    await getContainer().client().workspace.mcp.configure(params);
    await invalidateMcp();
  }, []);
}

/** Remove a server from the registry (workspace.mcp.remove). */
export function useRemoveMCPServer(): (name: string) => Promise<void> {
  return useCallback(async (name) => {
    await getContainer().client().workspace.mcp.remove(name);
    await invalidateMcp();
  }, []);
}

/** Flip a server's enablement without re-sending its whole config. */
export function useSetMCPServerEnabled(): (name: string, enabled: boolean) => Promise<void> {
  return useCallback(async (name, enabled) => {
    await getContainer().client().workspace.mcp.setEnabled(name, enabled);
    await invalidateMcp();
  }, []);
}

export interface MCPTestOutcome {
  ok: boolean;
  /** Human-readable failure reason (already flattened from ProblemData). */
  error?: string;
}

/**
 * Live-probe a server config (workspace.mcp.test) WITHOUT persisting it — the
 * runtime dials it once and reports back. A failed probe comes back as
 * `{ ok:false, error }` (NOT an RPC error), so callers render the reason inline
 * (mirrors useTestProvider).
 */
export function useTestMCPServer(): (params: ConfigureMCPServerRequest) => Promise<MCPTestOutcome> {
  return useCallback(async (params) => {
    const res = await getContainer().client().workspace.mcp.test(params);
    return {
      ok: res.ok,
      error: res.ok ? undefined : (errorDetail(res.error) ?? t("mcp.error.test")),
    };
  }, []);
}
