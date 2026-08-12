import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { mcpServerGateway } from "../application/ports/mcpServerGateway";
import { installMCPServerGateway } from "./runtimeMcpServerGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
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
    setContainer({ client: () => ({ mcp: { create } }) as unknown as LyraClient });
    uninstall = installMCPServerGateway();

    await expect(
      mcpServerGateway().create({
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
    setContainer({ client: () => ({ mcp: { update } }) as unknown as LyraClient });
    uninstall = installMCPServerGateway();

    await expect(mcpServerGateway().setEnabled("cloud", false)).resolves.toMatchObject({
      name: "cloud",
      status: "disabled",
      enabled: false,
      type: "streamableHttp",
    });
    expect(update).toHaveBeenCalledWith({ server: "cloud", enabled: false });
  });
});
