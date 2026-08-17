// Cutover slices — the cached app data providers that ride the JSON-RPC
// stack. Locks the full wiring (provider → container.methods() → client →
// transport) plus each v2 shape mapping:
//   - sessions:    Page<Session>.data → AgentSessionSummary (updatedAt → time)
//   - projects:    Page<WorkspaceSummary>.data → WorkspaceProjectSummary
//   - grep:        params pass-through, result verbatim (matches + total)
//   - file-head:   params pass-through, FileHead unwrapped to its lines

import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceFileLine,
  WorkspaceGrepResult,
  WorkspaceProjectSummary,
  WorkspaceDiff,
} from "@/plugins/builtin/workspace/public/queries";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createLyraClient, JSONRPC_VERSION } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import type { WireMethodName } from "@/rpc/wire.methods.generated";
import { defaultDataProviders } from "./index";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

afterEach(resetContainer);

// Run a provider against a scripted set of method → result responses. The
// provider may fan out; requests are answered in the order listed, which is
// also the fire order.
async function runProvider<T>(
  key: string,
  responses: Array<[method: WireMethodName, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createLyraClient(t);
  try {
    setContainer({ client: () => client });
    await loadPluginsForTest(defaultDataProviders);

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
  } finally {
    await client.close();
  }
}

describe("defaultDataProviders — providers over JSON-RPC", () => {
  it("rejects missing parameters before a parameterized provider reaches RPC", async () => {
    const client = createLyraClient(createMemoryTransport());
    try {
      setContainer({ client: () => client });
      await loadPluginsForTest(defaultDataProviders);

      for (const key of [
        "diff",
        "grep",
        "file-head",
        "skills",
        "skill-proposals",
        "agent-docs",
        "approval-rules",
        "list-files",
        "read-file",
      ]) {
        const fetcher = lookupDataProvider(key);
        expect(fetcher).toBeDefined();
        await expect(fetcher!()).rejects.toThrow(`Data provider "${key}" requires parameters`);
      }
    } finally {
      await client.close();
    }
  });

  it("workspace catalogs bind the selected project and retain proposal decision scope", async () => {
    const { value: skills, requests: skillRequests } = await runProvider<
      Array<{ name: string; scope: string }>
    >("skills", [["skills.discovered.list", { data: [{ name: "verify", scope: "project" }] }]], {
      cwd: "/work/alpha",
    });
    expect(skillRequests[0]?.params).toEqual({ workspace: { path: "/work/alpha" } });
    expect(skills).toEqual([{ name: "verify", description: "", scope: "project" }]);

    const { value: proposals, requests: proposalRequests } = await runProvider<
      Array<{ workspace: string; name: string }>
    >(
      "skill-proposals",
      [
        [
          "skills.proposals.list",
          {
            data: [
              {
                name: "verify",
                revision: "rev_1",
                scope: "project",
                description: "Verify changes",
                instructions: "Run the checks.",
              },
            ],
          },
        ],
      ],
      { cwd: "/work/beta" },
    );
    expect(proposalRequests[0]?.params).toEqual({ workspace: { path: "/work/beta" } });
    expect(proposals[0]).toMatchObject({ workspace: "/work/beta", name: "verify" });

    const { requests: docRequests } = await runProvider(
      "agent-docs",
      [["agentDocs.list", { data: [] }]],
      { cwd: "/work/gamma" },
    );
    expect(docRequests[0]?.params).toEqual({ workspace: { path: "/work/gamma" } });
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

  it("models: queries enabled providers only and maps their catalogs", async () => {
    const { value, requests } = await runProvider<
      Array<{ id: string; provider: string; label: string; multimodal: boolean }>
    >("models", [
      [
        "providers.list",
        {
          data: [
            { id: "openai", apiKeyMasked: "sk****42" },
            { id: "disabled", apiKeyMasked: "" },
          ],
        },
      ],
      [
        "models.list",
        {
          data: [
            {
              id: "gpt-test",
              provider: "openai",
              displayName: "GPT Test",
              capabilities: { multimodal: true },
            },
          ],
        },
      ],
    ]);

    expect(requests).toEqual([
      { method: "providers.list", params: {} },
      { method: "models.list", params: { provider: "openai" } },
    ]);
    expect(value).toEqual([
      { id: "gpt-test", provider: "openai", label: "GPT Test", multimodal: true },
    ]);
  });

  it("keeps one multi-stage provider read on its admitted client generation", async () => {
    const retiredTransport = createMemoryTransport();
    const retiredClient = createLyraClient(retiredTransport);
    const successorTransport = createMemoryTransport();
    const successorClient = createLyraClient(successorTransport);
    try {
      setContainer({ client: () => retiredClient });
      await loadPluginsForTest(defaultDataProviders);
      const fetcher = lookupDataProvider("models");
      if (!fetcher) throw new Error('no provider for "models"');

      const pending = fetcher();
      const providersRequest = await waitForRequest(retiredTransport, "providers.list");
      setContainer({ client: () => successorClient });
      respondSuccess(retiredTransport, providersRequest.id, {
        data: [{ id: "openai", apiKeyMasked: "sk****42" }],
      });

      await vi.waitFor(() => {
        expect(
          [...retiredTransport.outbox(), ...successorTransport.outbox()].some(
            ({ method }) => method === "models.list",
          ),
        ).toBe(true);
      });
      const retiredModelsRequest = retiredTransport
        .outbox()
        .find(({ method }) => method === "models.list");
      const successorModelsRequest = successorTransport
        .outbox()
        .find(({ method }) => method === "models.list");
      const actualTransport = retiredModelsRequest ? retiredTransport : successorTransport;
      const actualRequest = retiredModelsRequest ?? successorModelsRequest;
      if (!actualRequest) throw new Error("models.list was not requested");
      respondSuccess(actualTransport, actualRequest.id, { data: [] });
      await pending;

      expect(retiredModelsRequest).toBeDefined();
      expect(successorModelsRequest).toBeUndefined();

      const successorPending = fetcher();
      const successorProvidersRequest = await waitForRequest(successorTransport, "providers.list");
      respondSuccess(successorTransport, successorProvidersRequest.id, { data: [] });
      await successorPending;
      expect(
        retiredTransport.outbox().filter(({ method }) => method === "providers.list"),
      ).toHaveLength(1);
      expect(
        successorTransport.outbox().filter(({ method }) => method === "providers.list"),
      ).toHaveLength(1);
    } finally {
      await Promise.all([retiredClient.close(), successorClient.close()]);
    }
  });

  it("models: preserves a Runtime failure instead of presenting an empty catalog", async () => {
    const transport = createMemoryTransport();
    const client = createLyraClient(transport);
    setContainer({ client: () => client });
    await loadPluginsForTest(defaultDataProviders);
    const fetcher = lookupDataProvider("models");
    if (!fetcher) throw new Error('no provider for "models"');

    const pending = fetcher();
    const providersRequest = await waitForRequest(transport, "providers.list");
    respondSuccess(transport, providersRequest.id, {
      data: [{ id: "openai", apiKeyMasked: "sk****42" }],
    });
    const modelsRequest = await waitForRequest(transport, "models.list");
    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: modelsRequest.id,
      error: {
        code: -32603,
        message: "Internal error",
        data: { type: "internal_error" },
      },
    });

    await expect(pending).rejects.toMatchObject({ name: "RpcError" });
    await client.close();
  });
});
