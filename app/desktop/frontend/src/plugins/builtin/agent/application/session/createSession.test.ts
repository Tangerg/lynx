// useCreateSession spins up a backend session as a hidden draft, opens it,
// and (optionally) queues a first message. Locks that wiring + the failure
// path (returns null, no throw).

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient, Methods } from "@/rpc";
import { asSessionId } from "@/rpc";
import { useAgentSessionStore } from "@/state/agentSessionStore";
import { useCreateSession } from "./createSession";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

function stubCreate(create: Methods["sessions"]["create"]) {
  setContainer({ client: () => ({ sessions: { create } }) as unknown as LyraClient });
}

afterEach(() => {
  resetContainer();
  useAgentSessionStore.setState({
    activeSessionId: "",
    tabIds: [],
    draftSessionIds: new Set<string>(),
    pendingMessages: {},
  });
});

const fakeSession = (id: string) => ({
  id: asSessionId(id),
  title: "New session",
  status: "idle" as const,
  model: "gpt-4o",
  createdAt: "",
  updatedAt: "",
  metadata: {},
});

describe("useCreateSession", () => {
  it("creates a draft, opens it active, and queues the first message", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-1"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const id = await result.current({
      firstInput: [{ type: "text", text: "first message" }],
      firstRunOptions: { provider: "openai", model: "gpt-5" },
    });

    expect(id).toBe("new-1");
    const s = useAgentSessionStore.getState();
    expect(s.activeSessionId).toBe("new-1");
    expect(s.tabIds).toContain("new-1");
    expect(s.draftSessionIds.has("new-1")).toBe(true);
    expect(s.takePendingMessage("new-1")).toEqual({
      input: [{ type: "text", text: "first message" }],
      runOptions: { provider: "openai", model: "gpt-5" },
    });
  });

  it("forwards cwd so the session lands in the chosen project directory", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-cwd"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await result.current({ cwd: "/tmp/proj" });

    // Second arg is the AbortSignal.timeout guard (CREATE_TIMEOUT_MS).
    expect(create).toHaveBeenCalledWith({ cwd: "/tmp/proj" }, expect.any(AbortSignal));
  });

  it("creates an empty draft (no message) for the New button", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-2"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await result.current();

    const s = useAgentSessionStore.getState();
    expect(s.draftSessionIds.has("new-2")).toBe(true);
    expect(s.takePendingMessage("new-2")).toBeUndefined();
  });

  it("returns null + doesn't throw when create fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    stubCreate(vi.fn().mockRejectedValue(new Error("boom")));
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current({ firstInput: [{ type: "text", text: "x" }] })).resolves.toBeNull();
    expect(useAgentSessionStore.getState().activeSessionId).toBe("");
  });

  it("re-entrant calls join the in-flight create (double-click ≠ two sessions)", async () => {
    // sessions.create is a round-trip; a second "New" click inside that
    // window must not create a second backend session + tab.
    let release!: (v: ReturnType<typeof fakeSession>) => void;
    const create = vi.fn(() => new Promise<ReturnType<typeof fakeSession>>((r) => (release = r)));
    stubCreate(create as unknown as Methods["sessions"]["create"]);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const first = result.current();
    const second = result.current(); // joins, does not re-fire
    release(fakeSession("new-3"));

    expect(await first).toBe("new-3");
    expect(await second).toBe("new-3");
    expect(create).toHaveBeenCalledTimes(1);
    expect(useAgentSessionStore.getState().tabIds).toEqual(["new-3"]);
  });
});
