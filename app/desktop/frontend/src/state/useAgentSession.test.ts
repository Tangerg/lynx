// useAgentSession owns the agent driver lifecycle for one session. The
// regression locked here: send() is re-entrancy-safe in the window before
// run.started arrives. The steady-state guard (useChatSend reads run.running)
// only flips true a round-trip later, so without the synchronous `starting`
// latch a second Enter fires a second runs.start — two backend runs + an
// orphaned optimistic bubble whose localId is never relabeled.

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDriver } from "@/plugins/sdk";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
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
