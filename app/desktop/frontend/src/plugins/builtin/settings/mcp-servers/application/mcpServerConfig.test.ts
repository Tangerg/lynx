import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { MCP_SERVERS_KEY, type MCPServerSettings } from "./mcpServerQueries";
import { createMCPServer, deleteMCPServer, setMCPServerEnabled } from "./mcpServerConfig";
import type { MCPServerGateway } from "./ports/mcpServerGateway";
import { MCPServerMutationOwner } from "./mcpServerMutationOwner";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
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

let uninstall: (() => void) | undefined;

function installGateway(gateway: MCPServerGateway): void {
  const owner = MCPServerMutationOwner.install(gateway);
  uninstall = () => owner.dispose();
}

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [MCP_SERVERS_KEY] });
});

describe("MCP server configuration", () => {
  it("commits a created server into the owned collection", async () => {
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);
    const created = server({
      id: "local",
      name: "local",
      type: "stdio",
      command: "tool-server",
      url: undefined,
    });
    installGateway({
      create: vi.fn().mockResolvedValue(created),
    } as unknown as MCPServerGateway);

    await expect(
      createMCPServer({
        name: "local",
        transport: "stdio",
        enabled: true,
        command: "tool-server",
      }),
    ).resolves.toEqual(created);
    expect(queryClient.getQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY])).toEqual([
      server(),
      created,
    ]);
  });

  it("serializes changes to one server and commits the last response", async () => {
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);
    const first = deferred<MCPServerSettings>();
    const second = deferred<MCPServerSettings>();
    const setEnabled = vi
      .fn<(name: string, enabled: boolean) => Promise<MCPServerSettings>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    installGateway({ setEnabled } as unknown as MCPServerGateway);

    const disabled = setMCPServerEnabled("cloud", false);
    const enabled = setMCPServerEnabled("cloud", true);
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalledTimes(1));

    first.resolve(server({ status: "disabled", enabled: false }));
    await expect(disabled).resolves.toMatchObject({ enabled: false });
    await vi.waitFor(() => expect(setEnabled).toHaveBeenNthCalledWith(2, "cloud", true));

    second.resolve(server({ status: "connected", enabled: true, tools: 2, toolCount: 2 }));
    await expect(enabled).resolves.toMatchObject({ status: "connected", enabled: true });
    expect(queryClient.getQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY])?.[0]).toMatchObject({
      status: "connected",
      enabled: true,
      toolCount: 2,
    });
  });

  it("orders deletion after an in-flight update to the same server", async () => {
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);
    const first = deferred<MCPServerSettings>();
    const setEnabled = vi.fn().mockReturnValue(first.promise);
    const remove = vi.fn().mockResolvedValue(undefined);
    installGateway({
      setEnabled,
      delete: remove,
    } as unknown as MCPServerGateway);

    const disabled = setMCPServerEnabled("cloud", false);
    const deleted = deleteMCPServer("cloud");
    await Promise.resolve();
    expect(remove).not.toHaveBeenCalled();

    first.resolve(server({ status: "disabled", enabled: false }));
    await disabled;
    await deleted;
    expect(remove).toHaveBeenCalledWith("cloud");
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([]);
  });

  it("continues with a later server change after a rejected command", async () => {
    const first = deferred<MCPServerSettings>();
    const setEnabled = vi
      .fn<(name: string, enabled: boolean) => Promise<MCPServerSettings>>()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(server({ status: "connected" }));
    installGateway({ setEnabled } as unknown as MCPServerGateway);

    const rejected = setMCPServerEnabled("cloud", false);
    const accepted = setMCPServerEnabled("cloud", true);
    first.reject(new Error("not saved"));

    await expect(rejected).rejects.toThrow("not saved");
    await expect(accepted).resolves.toMatchObject({ status: "connected" });
    expect(setEnabled).toHaveBeenNthCalledWith(2, "cloud", true);
  });
});
