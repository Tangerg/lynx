// Cutover slices — the side-panel data providers that now ride the
// JSON-RPC stack instead of REST GET. Locks the full wiring (provider →
// container.methods() → client → transport) plus each shape mapping:
//   - sessions:    Page<Session> → SidebarSession (updatedAt → time)
//   - projects:    Project[] passthrough (structurally identical)
//   - mcp-servers: lean MCPServer → sidebar row (synthesised id + icon)

import type {
  MCPServer as SidebarMCPServer,
  SidebarProject,
  SidebarSession,
} from "@/lib/data/queries";
import { afterEach, describe, expect, it } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createMethods, createRpcClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import { defaultData } from "./index";

afterEach(resetContainer);

// Wire a memory-transport-backed container, load defaultData, then run the
// named provider while answering its single outbound RPC with `result`.
async function runProvider<T>(key: string, method: string, result: unknown): Promise<T> {
  const t = createMemoryTransport();
  setContainer({ methods: () => createMethods(createRpcClient(t)) });
  await loadPlugin(defaultData);

  const fetcher = lookupDataProvider<T>(key);
  if (!fetcher) throw new Error(`no provider for "${key}"`);
  const pending = fetcher();
  const req = await waitForRequest(t, method);
  respondSuccess(t, req.id, result);
  return pending;
}

describe("defaultData — providers over JSON-RPC", () => {
  it("sessions: maps Page<Session> into SidebarSession rows (updatedAt → time)", async () => {
    const rows = await runProvider<SidebarSession[]>("sessions", "sessions.list", {
      items: [
        {
          id: "s1",
          title: "Refactor auth",
          status: "running",
          model: "Sonnet 4.5",
          createdAt: "2026-05-29T00:00:00Z",
          updatedAt: "2026-05-29T01:00:00Z",
          metadata: {},
        },
      ],
      hasMore: false,
    });
    expect(rows).toEqual([
      {
        id: "s1",
        title: "Refactor auth",
        status: "running",
        model: "Sonnet 4.5",
        time: "2026-05-29T01:00:00Z",
      },
    ]);
  });

  it("projects: passes Project[] through unchanged", async () => {
    const rows = await runProvider<SidebarProject[]>("projects", "workspace.projects", [
      { id: "p1", name: "fern-api", branch: "feat/result-type", active: true },
    ]);
    expect(rows).toEqual([
      { id: "p1", name: "fern-api", branch: "feat/result-type", active: true },
    ]);
  });

  it("mcp-servers: synthesises id from name + maps name → icon", async () => {
    const rows = await runProvider<SidebarMCPServer[]>("mcp-servers", "workspace.mcp.list", [
      { name: "Git", desc: "Branches, commits", toolCount: 12, status: "active" },
      { name: "Unknown", desc: "novel server", toolCount: 1, status: "idle" },
    ]);
    expect(rows).toEqual([
      {
        id: "Git",
        name: "Git",
        desc: "Branches, commits",
        tools: 12,
        status: "active",
        icon: "branch",
      },
      {
        id: "Unknown",
        name: "Unknown",
        desc: "novel server",
        tools: 1,
        status: "idle",
        icon: "tool",
      },
    ]);
  });
});
