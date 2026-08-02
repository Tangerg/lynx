import { afterEach, describe, expect, it, vi } from "vitest";
import { authorizeMCPServer } from "./mcpServerConfig";
import {
  configureMCPServerGateway,
  type MCPAuthorizationAttempt,
  type MCPServerGateway,
} from "./ports/mcpServerGateway";

const inertGateway = {
  create: vi.fn(),
  update: vi.fn(),
  delete: vi.fn(),
  setEnabled: vi.fn(),
  test: vi.fn(),
} as const;

let disposeGateway: () => void = () => undefined;

afterEach(() => {
  disposeGateway();
  disposeGateway = () => undefined;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("MCP authorization attempts", () => {
  it("polls the created resource until it succeeds", async () => {
    vi.useFakeTimers();
    const getAuthorizationAttempt = vi
      .fn<(id: string) => Promise<MCPAuthorizationAttempt>>()
      .mockResolvedValueOnce({ id: "mcpauth_1", status: "pending" })
      .mockResolvedValueOnce({ id: "mcpauth_1", status: "succeeded" });
    disposeGateway = configureMCPServerGateway({
      ...inertGateway,
      createAuthorizationAttempt: vi.fn().mockResolvedValue({ id: "mcpauth_1", status: "pending" }),
      getAuthorizationAttempt,
    } as MCPServerGateway);

    const authorization = authorizeMCPServer("github");
    await vi.advanceTimersByTimeAsync(1_000);
    await authorization;

    expect(getAuthorizationAttempt).toHaveBeenNthCalledWith(1, "mcpauth_1", undefined);
    expect(getAuthorizationAttempt).toHaveBeenNthCalledWith(2, "mcpauth_1", undefined);
  });

  it("surfaces a failed terminal attempt without parsing transport errors", async () => {
    disposeGateway = configureMCPServerGateway({
      ...inertGateway,
      createAuthorizationAttempt: vi.fn().mockResolvedValue({
        id: "mcpauth_2",
        status: "failed",
        error: "Sign-in didn't complete. Try again.",
      }),
      getAuthorizationAttempt: vi.fn(),
    } as MCPServerGateway);

    await expect(authorizeMCPServer("github")).rejects.toThrow(
      "Sign-in didn't complete. Try again.",
    );
  });
});
