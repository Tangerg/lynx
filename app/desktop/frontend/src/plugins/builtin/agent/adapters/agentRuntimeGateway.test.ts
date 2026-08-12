import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { RpcError, type LyraClient, type Methods } from "@/rpc";
import { asRunId, asSegmentId, asSessionId } from "@/rpc";
import * as runtimeCapabilities from "@/plugins/builtin/runtime/public/capabilities";
import { agentRuntime } from "../application/ports/runtimeGateway";
import { installAgentRuntimeGateway } from "./agentRuntimeGateway";

let uninstall: (() => void) | undefined;

function autoPage<T>(data: T[]) {
  return { autoPagingToArray: vi.fn().mockResolvedValue(data) };
}

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  vi.restoreAllMocks();
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

  it("translates structured steering input only at the runtime adapter", async () => {
    const steer = vi.fn().mockResolvedValue({});
    setContainer({
      client: () => ({ runs: { steer } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await agentRuntime().steerRun("run_1", "seg_1", {
      parts: [
        { kind: "text", text: "compare this" },
        { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
      ],
    });

    expect(steer).toHaveBeenCalledWith(asRunId("run_1"), asSegmentId("seg_1"), [
      { type: "text", text: "compare this" },
      { type: "image", mime: "image/png", data: "aW1hZ2U=" },
    ] satisfies Parameters<Methods["runs"]["steer"]>[2]);
  });

  it.each([
    { supported: false, expected: undefined },
    { supported: true, expected: true },
  ])(
    "queries descendants explicitly when subagents supported=$supported",
    async ({ supported, expected }) => {
      vi.spyOn(runtimeCapabilities, "runtimeCapability").mockReturnValue(supported);
      const listRuns = vi.fn(() => autoPage([]));
      setContainer({
        client: () =>
          ({
            items: { list: vi.fn(() => autoPage([])) },
            runs: { list: listRuns },
            interrupts: { list: vi.fn(() => autoPage([])) },
            plan: {
              get: vi.fn().mockResolvedValue({
                type: "plan",
                sessionId: "ses_1",
                revision: 0,
                plan: [],
              }),
            },
          }) as unknown as LyraClient,
      });
      uninstall = installAgentRuntimeGateway();

      await agentRuntime().loadSessionSnapshot("ses_1");

      expect(listRuns).toHaveBeenCalledWith({
        sessionId: asSessionId("ses_1"),
        ...(expected ? { includeDescendants: expected } : {}),
      });
    },
  );

  it("translates an authoritatively missing session into an absent snapshot", async () => {
    const missing = new RpcError({
      code: -32002,
      message: "session missing",
      data: { type: "session_not_found" },
    });
    setContainer({
      client: () =>
        ({
          items: {
            list: vi.fn(() => ({
              autoPagingToArray: vi.fn().mockRejectedValue(missing),
            })),
          },
          runs: { list: vi.fn(() => autoPage([])) },
          interrupts: { list: vi.fn(() => autoPage([])) },
          plan: { get: vi.fn().mockRejectedValue(missing) },
        }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().loadSessionSnapshot("ses_gone")).resolves.toBeNull();
  });

  it("treats an already missing Session as a completed delete", async () => {
    const missing = new RpcError({
      code: -32002,
      message: "session missing",
      data: { type: "session_not_found" },
    });
    setContainer({
      client: () =>
        ({ sessions: { delete: vi.fn().mockRejectedValue(missing) } }) as unknown as LyraClient,
    });
    uninstall = installAgentRuntimeGateway();

    await expect(agentRuntime().deleteSession("ses_gone")).resolves.toBeUndefined();
  });
});
