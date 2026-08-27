import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import {
  authorizeMCPServer,
  createMCPServer,
  reconnectMCPServer,
  setMCPServerEnabled,
} from "../application/mcpServerConfig";
import { MCP_SERVERS_KEY, type MCPServerSettings } from "../application/mcpServerQueries";
import { installMCPServerGateway } from "./runtimeMcpServerGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [MCP_SERVERS_KEY] });
  vi.useRealTimers();
});

describe("runtimeMcpServerGateway", () => {
  it("maps the complete server returned by create", async () => {
    const create = vi.fn().mockResolvedValue({
      name: "local-tools",
      description: "Local tools",
      connection: { type: "stdio", command: "tool-server", args: ["--stdio"] },
      status: { type: "connected", toolCount: 3 },
      timeoutSeconds: 15,
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });
    setContainer({ client: () => ({ mcp: { create } }) as unknown as ScopeAppClient });
    uninstall = installMCPServerGateway().dispose;

    await expect(
      createMCPServer({
        name: "local-tools",
        transport: "stdio",
        enabled: true,
        command: "tool-server",
        args: ["--stdio"],
      }),
    ).resolves.toMatchObject({
      id: "local-tools",
      name: "local-tools",
      desc: "Local tools",
      tools: 3,
      status: "connected",
      type: "stdio",
      enabled: true,
      command: "tool-server",
      args: ["--stdio"],
      toolCount: 3,
    });
  });

  it("returns the stored server after an enablement change", async () => {
    const update = vi.fn().mockResolvedValue({
      name: "cloud",
      connection: { type: "streamableHttp", url: "https://example.test/mcp" },
      status: { type: "disabled" },
    });
    setContainer({ client: () => ({ mcp: { update } }) as unknown as ScopeAppClient });
    uninstall = installMCPServerGateway().dispose;

    await expect(setMCPServerEnabled("cloud", false)).resolves.toMatchObject({
      name: "cloud",
      status: "disabled",
      enabled: false,
      type: "streamableHttp",
    });
    expect(update).toHaveBeenCalledWith({ server: "cloud", enabled: false });
  });

  it("retires in-flight and queued server commands before installing a successor", async () => {
    const retiredUpdate = deferred<ReturnType<typeof runtimeServer>>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi
      .fn()
      .mockResolvedValue(runtimeServer({ status: { type: "connected", toolCount: 2 } }));
    setContainer({
      client: () => ({ mcp: { update: updateRetired } }) as unknown as ScopeAppClient,
    });
    const retiredInstallation = installMCPServerGateway();
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);

    const inFlight = setMCPServerEnabled("cloud", false);
    const queued = setMCPServerEnabled("cloud", true);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ mcp: { update: updateSuccessor } }) as unknown as ScopeAppClient,
    });
    const successorInstallation = installMCPServerGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "connected", tools: 2 })]);

    retiredUpdate.resolve(runtimeServer({ status: { type: "disabled" } }));
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([
      server({ status: "connected", tools: 2 }),
    ]);
  });

  it("does not continue an authorization attempt through a successor Runtime", async () => {
    vi.useFakeTimers();
    const createRetired = vi.fn().mockResolvedValue({
      id: "mcpauth_retired",
      status: { type: "pending" },
    });
    setContainer({
      client: () =>
        ({
          mcp: { authorizationAttempts: { create: createRetired } },
        }) as unknown as ScopeAppClient,
    });
    const retiredInstallation = installMCPServerGateway();
    const authorization = rejected(authorizeMCPServer("github"));
    await vi.waitFor(() => expect(createRetired).toHaveBeenCalledOnce());

    const getSuccessor = vi.fn().mockResolvedValue({
      id: "mcpauth_retired",
      status: { type: "succeeded" },
    });
    setContainer({
      client: () =>
        ({ mcp: { authorizationAttempts: { get: getSuccessor } } }) as unknown as ScopeAppClient,
    });
    const successorInstallation = installMCPServerGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    await vi.advanceTimersByTimeAsync(500);

    await expect(authorization).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(getSuccessor).not.toHaveBeenCalled();
  });

  it("binds reconnect to the exact Runtime client captured by its installation", async () => {
    const reconnectRetired = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectRetired } }) as unknown as ScopeAppClient,
    });
    uninstall = installMCPServerGateway().dispose;

    const reconnectSuccessor = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectSuccessor } }) as unknown as ScopeAppClient,
    });

    await reconnectMCPServer("cloud");

    expect(reconnectRetired).toHaveBeenCalledWith("cloud");
    expect(reconnectSuccessor).not.toHaveBeenCalled();
  });

  it("retires an admitted reconnect when a successor Host takes ownership", async () => {
    const retired = deferred<void>();
    const reconnectRetired = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectRetired } }) as unknown as ScopeAppClient,
    });
    const retiredInstallation = installMCPServerGateway();
    const reconnect = rejected(reconnectMCPServer("cloud"));
    await vi.waitFor(() => expect(reconnectRetired).toHaveBeenCalledOnce());

    const reconnectSuccessor = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectSuccessor } }) as unknown as ScopeAppClient,
    });
    const successorInstallation = installMCPServerGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };

    retired.resolve();
    await expect(reconnect).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(reconnectSuccessor).not.toHaveBeenCalled();
  });
});

function runtimeServer(overrides: Record<string, unknown> = {}) {
  return {
    name: "cloud",
    connection: { type: "streamableHttp" as const, url: "https://example.test/mcp" },
    status: { type: "disconnected" as const },
    ...overrides,
  };
}

function server(overrides: Partial<MCPServerSettings> = {}): MCPServerSettings {
  return {
    id: "cloud",
    name: "cloud",
    desc: "",
    tools: 0,
    status: "disconnected",
    icon: "tool",
    type: "streamableHttp",
    enabled: true,
    url: "https://example.test/mcp",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
