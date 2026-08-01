// Cutover slices — the cached app data providers that ride the JSON-RPC
// stack. Locks the full wiring (provider → container.methods() → client →
// transport) plus each v2 shape mapping:
//   - sessions:    Page<Session>.data → AgentSessionSummary (updatedAt → time)
//   - projects:    Page<WorkspaceSummary>.data → WorkspaceProjectSummary
//   - mcp-servers: enriched B3 entry → status summary (id + icon + inline toolCount)
//   - grep:        params pass-through, result verbatim (matches + total)
//   - file-head:   params pass-through, FileHead unwrapped to its lines

import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type { MCPServerSummary } from "@/plugins/builtin/settings/mcp-servers/public/queries";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceFileLine,
  WorkspaceGrepResult,
  WorkspaceProjectSummary,
  WorkspaceDiff,
} from "@/plugins/builtin/workspace/public/queries";
import { afterEach, describe, expect, it } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import type { WireMethodName } from "@/rpc/wire.methods.generated";
import { defaultDataProviders } from "./index";

afterEach(resetContainer);

// Run a provider against a scripted set of method → result responses. The
// provider may fan out (mcp-servers fires two calls in parallel); requests
// are answered in the order listed, which is also the fire order.
async function runProvider<T>(
  key: string,
  responses: Array<[method: WireMethodName, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createLyraClient(t);
  setContainer({ client: () => client });
  await loadPlugin(defaultDataProviders);

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

describe("defaultDataProviders — providers over JSON-RPC", () => {
  it("rejects missing parameters before a parameterized provider reaches RPC", async () => {
    const client = createLyraClient(createMemoryTransport());
    setContainer({ client: () => client });
    await loadPlugin(defaultDataProviders);

    for (const key of [
      "mcp-tools",
      "diff",
      "grep",
      "file-head",
      "approval-rules",
      "list-files",
      "read-file",
    ]) {
      const fetcher = lookupDataProvider(key);
      expect(fetcher).toBeDefined();
      await expect(fetcher!()).rejects.toThrow(`Data provider "${key}" requires parameters`);
    }
  });

  it("sessions: maps Page<Session>.data into AgentSessionSummary rows (updatedAt → time)", async () => {
    const { value: rows } = await runProvider<AgentSessionSummary[]>("sessions", [
      [
        "sessions.list",
        {
          data: [
            {
              id: "ses_1",
              revision: 7,
              title: "Refactor auth",
              status: "running",
              model: "claude",
              workspace: {
                ref: { path: "/work/auth" },
                projectRoot: "/work/auth",
                availability: "available",
              },
              createdAt: "2026-06-01T00:00:00Z",
              updatedAt: "2026-06-01T01:00:00Z",
            },
          ],
        },
      ],
    ]);
    expect(rows).toEqual([
      {
        id: "ses_1",
        revision: 7,
        title: "Refactor auth",
        status: "running",
        model: "claude",
        cwd: "/work/auth",
        time: "2026-06-01T01:00:00Z",
      },
    ]);
  });

  it("projects: maps WorkspaceSummary identity into workspace rows", async () => {
    const { value: rows } = await runProvider<WorkspaceProjectSummary[]>("projects", [
      [
        "workspaces.list",
        {
          data: [
            {
              workspace: {
                ref: { path: "/work/fern" },
                projectRoot: "/work/fern",
                availability: "available",
              },
              name: "fern-api",
              sessionCount: 3,
            },
          ],
        },
      ],
    ]);
    expect(rows).toEqual([
      {
        id: "/work/fern",
        name: "fern-api",
        sessionCount: 3,
      },
    ]);
  });

  it("mcp-servers: maps tool counts, five states, and localized inline errors", async () => {
    const { value: rows } = await runProvider<MCPServerSummary[]>("mcp-servers", [
      [
        "mcp.servers.list",
        {
          data: [
            { name: "Git", status: "connected", toolCount: 2, description: "Branches, commits" },
            {
              name: "Flaky",
              status: "failed",
              error: { type: "mcp_dial_failed" },
            },
            { name: "Cloud", status: "needsAuth", authStatus: "notLoggedIn" },
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
        status: "connected",
        errorDetail: undefined,
        icon: "branch",
      },
      {
        id: "Flaky",
        name: "Flaky",
        desc: "",
        tools: 0,
        status: "failed",
        errorDetail: "Couldn't reach this server — check the command or URL and retry.",
        icon: "tool",
      },
      {
        id: "Cloud",
        name: "Cloud",
        desc: "",
        tools: 0,
        status: "needsAuth",
        errorDetail: undefined,
        icon: "tool",
      },
    ]);
  });

  it("files-changed: forwards cwd, maps statuses, keeps ± counts / binary honest", async () => {
    const { value: rows, requests } = await runProvider<WorkspaceFileChangeSummary[]>(
      "files-changed",
      [
        [
          "workspace.changes.list",
          {
            data: [
              { path: "src/a.ts", status: "modified", added: 3, removed: 1 },
              { path: "logo.png", status: "untracked", binary: true }, // no fabricated ±0
            ],
          },
        ],
      ],
      { cwd: "/work/auth" },
    );
    expect(requests[0]?.params).toEqual({ workspace: { path: "/work/auth" } });
    expect(rows).toEqual([
      { path: "src/a.ts", change: "mod", added: 3, removed: 1, binary: undefined },
      { path: "logo.png", change: "add", added: undefined, removed: undefined, binary: true },
    ]);
  });

  it("diff: pins format=rows on the wire and defaults files to []", async () => {
    const { value, requests } = await runProvider<WorkspaceDiff>(
      "diff",
      [["workspace.diff.get", { truncated: true }]], // rows response may omit files
      { cwd: "/work/auth", path: "src/a.ts", mode: "worktree" },
    );
    expect(requests[0]?.params).toEqual({
      path: "src/a.ts",
      mode: "worktree",
      format: "rows",
      workspace: { path: "/work/auth" },
    });
    expect(value).toEqual({ files: [], truncated: true });
  });

  it("grep: forwards params on the wire and returns matches + total verbatim", async () => {
    const result: WorkspaceGrepResult = {
      matches: [{ path: "src/a.ts", lineNumber: 12, text: "const x = 1" }],
      total: 5, // > matches.length — the server-truncation signal must survive
    };
    const { value, requests } = await runProvider<WorkspaceGrepResult>(
      "grep",
      [["workspace.files.search", result]],
      { cwd: "/work/auth", query: "const x", limit: 1 },
    );
    expect(requests[0]?.params).toEqual({
      query: "const x",
      limit: 1,
      workspace: { path: "/work/auth" },
    });
    expect(value).toEqual(result);
  });

  it("file-head: forwards params and unwraps FileHead to its lines", async () => {
    const { value, requests } = await runProvider<WorkspaceFileLine[]>(
      "file-head",
      [
        [
          "workspace.files.head",
          { path: "src/a.ts", lines: [{ lineNumber: 1, text: "import x" }] },
        ],
      ],
      { cwd: "/work/auth", path: "src/a.ts", lines: 40 },
    );
    expect(requests[0]?.params).toEqual({
      path: "src/a.ts",
      lines: 40,
      workspace: { path: "/work/auth" },
    });
    expect(value).toEqual([{ lineNumber: 1, text: "import x" }]);
  });
});
