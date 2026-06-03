// Cutover slices — the side-panel data providers that ride the JSON-RPC
// stack. Locks the full wiring (provider → container.methods() → client →
// transport) plus each v2 shape mapping:
//   - sessions:    Page<Session>.data → SidebarSession (updatedAt → time)
//   - projects:    Project[] (cwd identity) → SidebarProject (cwd → id)
//   - mcp-servers: McpServer → sidebar row (synthesised id + icon + status)

import type {
  MCPServer as SidebarMCPServer,
  SidebarProject,
  SidebarSession,
} from "@/lib/data/queries";
import { afterEach, describe, expect, it } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import { defaultData } from "./index";

afterEach(resetContainer);

async function runProvider<T>(key: string, method: string, result: unknown): Promise<T> {
  const t = createMemoryTransport();
  const client = createLyraClient(t);
  setContainer({ client: () => client });
  await loadPlugin(defaultData);

  const fetcher = lookupDataProvider<T>(key);
  if (!fetcher) throw new Error(`no provider for "${key}"`);
  const pending = fetcher();
  const req = await waitForRequest(t, method);
  respondSuccess(t, req.id, result);
  return pending;
}

describe("defaultData — providers over JSON-RPC", () => {
  it("sessions: maps Page<Session>.data into SidebarSession rows (updatedAt → time)", async () => {
    const rows = await runProvider<SidebarSession[]>("sessions", "sessions.list", {
      data: [
        {
          id: "ses_1",
          title: "Refactor auth",
          status: "running",
          model: "claude",
          cwd: "/work/auth",
          createdAt: "2026-06-01T00:00:00Z",
          updatedAt: "2026-06-01T01:00:00Z",
          metadata: {},
        },
      ],
    });
    expect(rows).toEqual([
      {
        id: "ses_1",
        title: "Refactor auth",
        status: "running",
        model: "claude",
        time: "2026-06-01T01:00:00Z",
      },
    ]);
  });

  it("projects: maps v2 Project (cwd identity) into SidebarProject rows", async () => {
    const rows = await runProvider<SidebarProject[]>("projects", "workspace.listProjects", [
      { cwd: "/work/fern", name: "fern-api", branch: "feat/result-type", sessionCount: 3 },
    ]);
    expect(rows).toEqual([{ id: "/work/fern", name: "fern-api", branch: "feat/result-type" }]);
  });

  it("mcp-servers: synthesises id from name + maps status + icon", async () => {
    const rows = await runProvider<SidebarMCPServer[]>("mcp-servers", "workspace.mcp.listServers", [
      { name: "Git", status: "connected", description: "Branches, commits" },
      { name: "Unknown", status: "disconnected" },
    ]);
    expect(rows).toEqual([
      {
        id: "Git",
        name: "Git",
        desc: "Branches, commits",
        tools: 0,
        status: "active",
        icon: "branch",
      },
      { id: "Unknown", name: "Unknown", desc: "", tools: 0, status: "idle", icon: "tool" },
    ]);
  });
});
