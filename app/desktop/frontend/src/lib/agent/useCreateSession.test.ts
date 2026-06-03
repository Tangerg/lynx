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
import { useSessionStore } from "@/state/sessionStore";
import { useCreateSession } from "./useCreateSession";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

function stubCreate(create: Methods["sessions"]["create"]) {
  setContainer({ client: () => ({ sessions: { create } }) as unknown as LyraClient });
}

afterEach(() => {
  resetContainer();
  useSessionStore.setState({
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

    const id = await result.current("first message");

    expect(id).toBe("new-1");
    const s = useSessionStore.getState();
    expect(s.activeSessionId).toBe("new-1");
    expect(s.tabIds).toContain("new-1");
    expect(s.draftSessionIds.has("new-1")).toBe(true);
    expect(s.takePendingMessage("new-1")).toBe("first message");
  });

  it("creates an empty draft (no message) for the New button", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-2"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await result.current();

    const s = useSessionStore.getState();
    expect(s.draftSessionIds.has("new-2")).toBe(true);
    expect(s.takePendingMessage("new-2")).toBeUndefined();
  });

  it("returns null + doesn't throw when create fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    stubCreate(vi.fn().mockRejectedValue(new Error("boom")));
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current("x")).resolves.toBeNull();
    expect(useSessionStore.getState().activeSessionId).toBe("");
  });
});
