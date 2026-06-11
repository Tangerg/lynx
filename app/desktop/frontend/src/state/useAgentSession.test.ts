// useAgentSession owns the agent driver lifecycle for one session. The
// regression locked here: send() is re-entrancy-safe in the window before
// run.started arrives. The steady-state guard (useChatSend reads run.running)
// only flips true a round-trip later, so without the synchronous `starting`
// latch a second Enter fires a second runs.start — two backend runs + an
// orphaned optimistic bubble whose localId is never relabeled.

import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDriver } from "@/plugins/sdk";
import type { LyraClient } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { resetContainer, setContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { useSessionStore } from "./sessionStore";
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
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
  // Mark draft so the effect skips history hydration (items.list → container).
  useSessionStore.setState({ draftSessionIds: new Set([SID]), activeSessionId: SID });
});
afterEach(() => {
  useAgentStore.getState().dropSession(SID);
  useSessionStore.setState({ draftSessionIds: new Set() });
});

describe("useAgentSession send re-entrancy", () => {
  it("ignores a second send before the first run starts (no duplicate run/bubble)", () => {
    const { driver, start } = parkedDriver();
    renderHook(() => useAgentSession(() => driver, SID));

    act(() => {
      const send = useAgentStore.getState().sessions[SID]!.send!;
      send("first");
      send("second"); // blocked by the starting latch — still pre-run.started
    });

    expect(start).toHaveBeenCalledTimes(1);
    const msgs = useAgentStore.getState().sessions[SID]!.view.messages;
    expect(msgs).toHaveLength(1);
    expect(msgs[0]!.blocks.some((b) => "text" in b && b.text === "first")).toBe(true);
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
    payload: { tool: { name: "bash", arguments: { command: "rm -rf build" } } },
  };

  function stubClient(overrides: Record<string, unknown> = {}) {
    const subscribe = vi.fn(() =>
      Promise.resolve({
        result: { runId: "run_live" },
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
    useSessionStore.setState({ draftSessionIds: new Set(), activeSessionId: RID });
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
            parentRunId: "run_int",
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
      expect(useAgentStore.getState().sessions[RID]!.view.openInterrupts).toHaveLength(1);
    });
    const view = useAgentStore.getState().sessions[RID]!.view;
    expect(view.openInterrupts[0]!.parentRunId).toBe("run_int");
    const approval = view.messages
      .flatMap((m) => m.blocks)
      .find((b) => b.kind === "approval" && b.itemId === "item_appr");
    expect(approval).toMatchObject({ status: "requires-action", parentRunId: "run_int" });
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
