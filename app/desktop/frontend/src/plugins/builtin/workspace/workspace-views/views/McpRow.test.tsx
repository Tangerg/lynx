import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MCPServerSettings } from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { notifyError } from "@/plugins/sdk";
import { McpRow } from "./McpRow";

const mcp = vi.hoisted(() => ({
  reconnect: vi.fn<(server: string) => Promise<void>>(),
  retired: new Error("mcp_server_mutation_generation_retired"),
}));

vi.mock("@/plugins/builtin/settings/mcp-servers/public/serverCatalog", async (importOriginal) => ({
  ...(await importOriginal<
    typeof import("@/plugins/builtin/settings/mcp-servers/public/serverCatalog")
  >()),
  reconnectMCPServer: mcp.reconnect,
  mcpServerMutationWasRetired: (error: unknown) => error === mcp.retired,
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  notifyError: vi.fn(),
}));

afterEach(() => {
  cleanup();
  mcp.reconnect.mockReset();
  vi.mocked(notifyError).mockClear();
});

describe("McpRow", () => {
  it("admits only one reconnect while the exact server command is unsettled", async () => {
    const admitted = deferred<void>();
    mcp.reconnect.mockImplementation(() => admitted.promise);
    render(<McpRow server={server()} />);

    const button = screen.getByRole("button", { name: "Reconnect" });
    fireEvent.click(button);
    fireEvent.click(button);

    await vi.waitFor(() => expect(mcp.reconnect).toHaveBeenCalledOnce());
    expect((button as HTMLButtonElement).disabled).toBe(true);

    await act(async () => admitted.resolve());
    await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    expect(notifyError).not.toHaveBeenCalled();
  });

  it("treats Runtime generation retirement as neutral settlement", async () => {
    const admitted = deferred<void>();
    mcp.reconnect.mockImplementation(() => admitted.promise);
    render(<McpRow server={server()} />);

    const button = screen.getByRole("button", { name: "Reconnect" });
    fireEvent.click(button);
    await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(true));

    await act(async () => admitted.reject(mcp.retired));

    await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    expect(notifyError).not.toHaveBeenCalled();
  });
});

function server(overrides: Partial<MCPServerSettings> = {}): MCPServerSettings {
  return {
    id: "cloud",
    name: "Cloud",
    desc: "Cloud tools",
    tools: 2,
    status: "disconnected",
    icon: "tool",
    type: "streamableHttp",
    enabled: true,
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}
