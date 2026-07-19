import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient, Methods } from "@/rpc";
import { asSessionId } from "@/rpc";
import { agentRuntime } from "../application/ports/runtimeGateway";
import { installAgentRuntimeGateway } from "./agentRuntimeGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
});

describe("agentRuntimeGateway", () => {
  it("forwards the caller snapshot revision without a get-before-write", async () => {
    const get = vi.fn();
    const update = vi.fn().mockResolvedValue({ revision: 12 });
    setContainer({
      client: () => ({ sessions: { get, update } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(
      agentRuntime().updateSession({
        sessionId: "ses_1",
        expectedRevision: 11,
        favorite: true,
      }),
    ).resolves.toEqual({ revision: 12 });

    expect(update).toHaveBeenCalledWith({
      sessionId: asSessionId("ses_1"),
      expectedRevision: 11,
      favorite: true,
    } satisfies Parameters<Methods["sessions"]["update"]>[0]);
    expect(get).not.toHaveBeenCalled();
  });
});
