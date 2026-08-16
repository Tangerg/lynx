import { afterEach, describe, expect, it } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createLyraClient, JSONRPC_VERSION } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import type { MCPServerSettings, MCPToolSummary } from "../application/mcpServerQueries";
import { registerMCPDataProviders } from "./runtimeMcpDataProviders";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const mcpDataProviders = definePlugin({
  name: "test.mcp-data-providers",
  setup(ctx) {
    registerMCPDataProviders(ctx);
  },
});

const clients: Array<ReturnType<typeof createLyraClient>> = [];

function testClient(transport: ReturnType<typeof createMemoryTransport>) {
  const client = createLyraClient(transport);
  clients.push(client);
  return client;
}

afterEach(async () => {
  await Promise.all(clients.splice(0).map((client) => client.close()));
  await resetContainer();
});

async function provider<T>(key: string): Promise<(params?: unknown) => Promise<T>> {
  await loadPluginsForTest(mcpDataProviders);
  const fetcher = lookupDataProvider<T>(key);
  if (!fetcher) throw new Error(`no provider for "${key}"`);
  return fetcher;
}

describe("runtime MCP data providers", () => {
  it("maps unified configuration, lifecycle, and localized inline errors", async () => {
    const transport = createMemoryTransport();
    const client = testClient(transport);
    setContainer({ client: () => client });
    const fetcher = await provider<MCPServerSettings[]>("mcp-servers");

    const pending = fetcher();
    const request = await waitForRequest(transport, "mcp.servers.list");
    respondSuccess(transport, request.id, {
      data: [
        {
          name: "Git",
          description: "Branches, commits",
          connection: { type: "stdio", command: "mcp-git" },
          status: { type: "connected", toolCount: 2 },
        },
        {
          name: "Flaky",
          connection: { type: "stdio", command: "mcp-flaky" },
          status: { type: "failed", error: { type: "mcp_dial_failed" } },
        },
        {
          name: "Cloud",
          connection: { type: "streamableHttp", url: "https://mcp.example/rpc" },
          status: { type: "needsAuth", error: { type: "mcp_authorization_required" } },
        },
      ],
    });

    await expect(pending).resolves.toMatchObject([
      {
        id: "Git",
        desc: "Branches, commits",
        tools: 2,
        status: "connected",
        icon: "branch",
        type: "stdio",
        enabled: true,
        command: "mcp-git",
        toolCount: 2,
      },
      {
        id: "Flaky",
        tools: 0,
        status: "failed",
        errorDetail: "Couldn't reach this server — check the command or URL and retry.",
        enabled: true,
      },
      {
        id: "Cloud",
        tools: 0,
        status: "needsAuth",
        errorDetail: "This server needs you to sign in before it can be used.",
        type: "streamableHttp",
        url: "https://mcp.example/rpc",
      },
    ]);
  });

  it("requires an explicit server and maps tool descriptions", async () => {
    const transport = createMemoryTransport();
    const client = testClient(transport);
    setContainer({ client: () => client });
    const fetcher = await provider<MCPToolSummary[]>("mcp-tools");

    await expect(fetcher()).rejects.toThrow('Data provider "mcp-tools" requires parameters');
    const pending = fetcher({ server: "Git" });
    const request = await waitForRequest(transport, "mcp.tools.list");
    expect(request.params).toEqual({ server: "Git" });
    respondSuccess(transport, request.id, {
      data: [
        { server: "Git", name: "status" },
        { server: "Git", name: "log", description: "Read history" },
      ],
    });

    await expect(pending).resolves.toEqual([
      { name: "status", description: "" },
      { name: "log", description: "Read history" },
    ]);
  });

  it("treats an unnegotiated optional MCP capability as an empty catalog", async () => {
    const transport = createMemoryTransport();
    const client = testClient(transport);
    setContainer({ client: () => client });
    const fetcher = await provider<MCPServerSettings[]>("mcp-servers");

    const pending = fetcher();
    const request = await waitForRequest(transport, "mcp.servers.list");
    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: request.id,
      error: {
        code: -32601,
        message: "Capability not negotiated",
        data: {
          type: "capability_not_negotiated",
          requiredCapabilities: [{ type: "feature", name: "mcp" }],
        },
      },
    });

    await expect(pending).resolves.toEqual([]);
  });
});
