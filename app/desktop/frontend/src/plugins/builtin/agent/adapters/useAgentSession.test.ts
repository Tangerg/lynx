// useAgentSession owns the agent driver lifecycle for one session. The
// regression locked here: send() is re-entrancy-safe in the window before
// run.started arrives. The steady-state guard (useChatSend reads run.running)
// only flips true a round-trip later, so without the synchronous `starting`
// latch a second Enter fires a second runs.start — two backend runs + an
// orphaned optimistic bubble whose localId is never relabeled.

import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDriver } from "@/plugins/sdk/types";
import { RpcError, type LyraClient, type RunEvent } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { agentTextInput } from "@/plugins/builtin/agent/domain/input";
import { resetContainer, setContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentSession } from "./useAgentSession";

const SID = "ses_dbl";

// A driver whose start() never resolves — keeps begin() parked in the
// pre-run.started window where the latch is the only guard.
function parkedDriver(): { driver: AgentDriver; start: ReturnType<typeof vi.fn> } {
  const start = vi.fn(() => new Promise<never>(() => {}));
  const resume = vi.fn(() => new Promise<never>(() => {}));
  return { driver: { start, resume } as unknown as AgentDriver, start };
}

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
  // Mark draft so the effect skips history hydration (items.list → container).
  useAgentSessionStore.setState({ draftSessionIds: new Set([SID]), activeSessionId: SID });
});
afterEach(() => {
  useAgentStore.getState().dropSession(SID);
  useAgentSessionStore.setState({ draftSessionIds: new Set() });
  resetContainer();
  vi.restoreAllMocks();
});

describe("useAgentSession send re-entrancy", () => {
  it("ignores a second send before the first run starts (no duplicate run/bubble)", () => {
    const { driver, start } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      const send = useAgentStore.getState().sessions[SID]!.send!;
      send(agentTextInput("first"));
      send(agentTextInput("second")); // blocked by the starting latch — still pre-run.started
    });

    expect(start).toHaveBeenCalledTimes(1);
    const msgs = useAgentStore.getState().sessions[SID]!.view.messages;
    expect(msgs).toHaveLength(1);
    expect(msgs[0]!.blocks.some((b) => "text" in b && b.text === "first")).toBe(true);
  });
});

describe("useAgentSession run timing guards", () => {
  it("surfaces synchronous driver failures and releases the start latch", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const start = vi
      .fn()
      .mockImplementationOnce(() => {
        throw new RpcError({
          code: -32002,
          message: "session missing",
          data: { type: "session_not_found", detail: "gone" },
        });
      })
      .mockImplementationOnce(() => new Promise<never>(() => {}));
    const driver = {
      start,
      resume: vi.fn(() => new Promise<never>(() => {})),
    } as unknown as AgentDriver;
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      useAgentStore.getState().sessions[SID]!.send!(agentTextInput("first"));
    });

    await waitFor(() => {
      expect(useAgentStore.getState().sessions[SID]!.view.error).toMatchObject({
        message: "gone",
        code: "session_not_found",
      });
    });

    act(() => {
      useAgentStore.getState().sessions[SID]!.send!(agentTextInput("second"));
    });

    expect(start).toHaveBeenCalledTimes(2);
  });

  it("ignores a second resume while the first continuation is still starting", () => {
    const resume = vi.fn(() => new Promise<never>(() => {}));
    const driver = {
      start: vi.fn(() => new Promise<never>(() => {})),
      resume,
    } as unknown as AgentDriver;
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      const resumeAction = useAgentStore.getState().sessions[SID]!.resume!;
      resumeAction("run_parent" as never, []);
      resumeAction("run_parent" as never, []);
    });

    expect(resume).toHaveBeenCalledTimes(1);
  });

  it("treats aborting an accepted stream as cancellation, not a start failure", async () => {
    const cancel = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () =>
        ({
          runs: { cancel },
        }) as unknown as LyraClient,
    });
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const onSettled = vi.fn();
    const onStartError = vi.fn();
    const resume = vi.fn((_: unknown, __: unknown, signal: AbortSignal) =>
      Promise.resolve({
        result: { runId: "run_resume" },
        events: abortRejectingEvents(signal),
      }),
    );
    const driver = {
      start: vi.fn(() => new Promise<never>(() => {})),
      resume,
    } as unknown as AgentDriver;
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      useAgentStore.getState().sessions[SID]!.resume!(
        "run_parent" as never,
        [],
        onSettled,
        onStartError,
      );
    });

    await waitFor(() => expect(onSettled).toHaveBeenCalledTimes(1));
    errorSpy.mockClear();

    act(() => {
      useAgentStore.getState().sessions[SID]!.stop?.();
    });

    await waitFor(() => expect(cancel).toHaveBeenCalledWith("run_resume"));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(onStartError).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });
});

// Durable recovery (API.md §10.2): opening a NON-draft session must rebuild
// unresolved HITL cards from runs.listOpenInterrupts and reattach to a
// still-running run via runs.subscribe — the two paths that make a reload
// survivable.
describe("useAgentSession durable recovery", () => {
  const RID = "ses_recover";

  const page = <T>(data: T[]) => ({ data });
  const approvalInterrupt = {
    type: "approval" as const,
    itemId: "item_appr",
    payload: { tool: { name: "shell", arguments: { command: "rm -rf build" } } },
  };

  function stubClient(overrides: Record<string, unknown> = {}) {
    const subscribe = vi.fn(() =>
      Promise.resolve({
        result: { runId: "run_live", segmentId: "seg_live" },
        // Parked stream — yields nothing, never ends (the run is "still going").
        events: (async function* () {
          yield* [];
          await new Promise<never>(() => {});
        })(),
      }),
    );
    setContainer({
      client: () =>
        ({
          items: { list: vi.fn().mockResolvedValue(page([])) },
          runs: {
            listOpenInterrupts: vi.fn().mockResolvedValue(page([])),
            list: vi.fn().mockResolvedValue(page([])),
            subscribe,
            ...(overrides as object),
          },
        }) as unknown as LyraClient,
    });
    return { subscribe };
  }

  beforeEach(() => {
    // NOT a draft — recovery only runs for existing sessions.
    useAgentSessionStore.setState({ draftSessionIds: new Set(), activeSessionId: RID });
  });
  afterEach(() => {
    useAgentStore.getState().dropSession(RID);
    resetContainer();
  });

  it("rebuilds pending approval cards from runs.listOpenInterrupts", async () => {
    stubClient({
      listOpenInterrupts: vi.fn().mockResolvedValue(
        page([
          {
            runId: "run_int",
            sessionId: RID,
            interrupts: [approvalInterrupt],
            createdAt: "2026-06-11T00:00:00Z",
          },
        ]),
      ),
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(useAgentStore.getState().sessions[RID]!.view.pendingInterrupts).toHaveLength(1);
    });
    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(view.pendingInterrupts[0]!.runId).toBe("run_int");
    const approval = view.messages
      .flatMap((m) => m.blocks)
      .find((b) => b.kind === "approval" && b.itemId === "item_appr");
    expect(approval).toMatchObject({ status: "requires-action", runId: "run_int" });
    expect(view.run.running).toBe(false); // interrupt = run already ended
  });

  it("reattaches to a still-running root run via runs.subscribe", async () => {
    const { subscribe } = stubClient({
      list: vi.fn().mockResolvedValue(
        page([
          { id: "run_sub", sessionId: RID, spawnedByItemId: "item_x" }, // subagent — skip
          { id: "run_live", sessionId: RID },
        ]),
      ),
    });
    const { driver } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, RID));

    await waitFor(() => {
      expect(useAgentStore.getState().sessions[RID]!.view.run.running).toBe(true);
    });
    expect(subscribe).toHaveBeenCalledTimes(1);
    expect(subscribe).toHaveBeenCalledWith("run_live", expect.any(AbortSignal));
    expect(useAgentStore.getState().sessions[RID]!.view.run.runId).toBe("run_live");
  });
});

function abortRejectingEvents(signal: AbortSignal): AsyncIterable<RunEvent> {
  return {
    [Symbol.asyncIterator]() {
      return {
        async next(): Promise<IteratorResult<RunEvent>> {
          await new Promise<never>((_, reject) => {
            if (signal.aborted) {
              reject(new Error("aborted"));
              return;
            }
            signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true });
          });
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}
