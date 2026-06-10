// Cutover slices — the side-panel data providers that ride the JSON-RPC
// stack. Locks the full wiring (provider → container.methods() → client →
// transport) plus each v2 shape mapping:
//   - sessions:    Page<Session>.data → SidebarSession (updatedAt → time)
//   - projects:    Page<Project>.data (cwd identity) → SidebarProject (cwd → id)
//   - mcp-servers: listServers ⨝ listTools → sidebar row (id + icon + tool count)
//   - grep:        params pass-through, result verbatim (matches + total)
//   - file-head:   params pass-through, FileHead unwrapped to its lines

import type {
  FileLine,
  GrepResult,
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

// Run a provider against a scripted set of method → result responses. The
// provider may fan out (mcp-servers fires two calls in parallel); requests
// are answered in the order listed, which is also the fire order.
async function runProvider<T>(
  key: string,
  responses: Array<[method: string, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createLyraClient(t);
  setContainer({ client: () => client });
  await loadPlugin(defaultData);

  const fetcher = lookupDataProvider<T>(key);
  if (!fetcher) throw new Error(`no provider for "${key}"`);
  const pending = fetcher(params);
  const requests: Array<{ method: string; params: unknown }> = [];
  for (const [method, result] of responses) {
    const req = await waitForRequest(t, method);
    requests.push({ method: req.method, params: req.params });
    respondSuccess(t, req.id, result);
  }
  return { value: await pending, requests };
}

describe("defaultData — providers over JSON-RPC", () => {
  it("sessions: maps Page<Session>.data into SidebarSession rows (updatedAt → time)", async () => {
    const { value: rows } = await runProvider<SidebarSession[]>("sessions", [
      [
        "sessions.list",
        {
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
        },
      ],
    ]);
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
    const { value: rows } = await runProvider<SidebarProject[]>("projects", [
      [
        "workspace.listProjects",
        {
          data: [
            { cwd: "/work/fern", name: "fern-api", branch: "feat/result-type", sessionCount: 3 },
          ],
        },
      ],
    ]);
    expect(rows).toEqual([{ id: "/work/fern", name: "fern-api", branch: "feat/result-type" }]);
  });

  it("mcp-servers: joins listTools counts onto listServers rows (id + icon + status)", async () => {
    const { value: rows } = await runProvider<SidebarMCPServer[]>("mcp-servers", [
      [
        "workspace.mcp.listServers",
        {
          data: [
            { name: "Git", status: "connected", description: "Branches, commits" },
            { name: "Unknown", status: "disconnected" },
          ],
        },
      ],
      [
        "workspace.mcp.listTools",
        {
          data: [
            { server: "Git", name: "log" },
            { server: "Git", name: "blame" },
            { server: "Orphan", name: "ghost" }, // unlisted server — must not crash the join
          ],
        },
      ],
    ]);
    expect(rows).toEqual([
      {
        id: "Git",
        name: "Git",
        desc: "Branches, commits",
        tools: 2,
        status: "active",
        icon: "branch",
      },
      { id: "Unknown", name: "Unknown", desc: "", tools: 0, status: "idle", icon: "tool" },
    ]);
  });

  it("grep: forwards params on the wire and returns matches + total verbatim", async () => {
    const result: GrepResult = {
      matches: [{ path: "src/a.ts", lineNumber: 12, text: "const x = 1" }],
      total: 5, // > matches.length — the server-truncation signal must survive
    };
    const { value, requests } = await runProvider<GrepResult>(
      "grep",
      [["workspace.grep", result]],
      { query: "const x", limit: 1 },
    );
    expect(requests[0]?.params).toEqual({ query: "const x", limit: 1 });
    expect(value).toEqual(result);
  });

  it("file-head: forwards params and unwraps FileHead to its lines", async () => {
    const { value, requests } = await runProvider<FileLine[]>(
      "file-head",
      [
        [
          "workspace.getFileHead",
          { path: "src/a.ts", lines: [{ lineNumber: 1, text: "import x" }] },
        ],
      ],
      { path: "src/a.ts", lines: 40 },
    );
    expect(requests[0]?.params).toEqual({ path: "src/a.ts", lines: 40 });
    expect(value).toEqual([{ lineNumber: 1, text: "import x" }]);
  });
});
