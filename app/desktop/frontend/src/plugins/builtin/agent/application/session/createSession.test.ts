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
import { agentTextInput } from "../../domain/input";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
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
    openSessionIds: [],
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
});

describe("useCreateSession", () => {
  it("creates a draft, opens it active, and queues the first message", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-1"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const id = await result.current({
      firstInput: agentTextInput("first message"),
      firstRunOptions: { provider: "openai", model: "gpt-5" },
    });

    expect(id).toBe("new-1");
    const s = useAgentSessionStore.getState();
    expect(s.activeSessionId).toBe("new-1");
    expect(s.openSessionIds).toContain("new-1");
    expect(s.draftSessionIds.has("new-1")).toBe(true);
    expect(s.takePendingMessage("new-1")).toEqual({
      input: agentTextInput("first message"),
      runOptions: { provider: "openai", model: "gpt-5" },
    });
  });

  it("wraps the chosen directory in a workspace reference", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-cwd"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await result.current({ cwd: "/tmp/proj" });

    // Second arg is the AbortSignal.timeout guard (CREATE_TIMEOUT_MS).
    expect(create).toHaveBeenCalledWith(
      { workspace: { path: "/tmp/proj" } },
      expect.any(AbortSignal),
    );
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

  it("reuses the fresh draft the user is already looking at", async () => {
    // Pressing New on the empty-composer screen asks for a destination that is
    // already on screen — creating there mints a second backend session and
    // orphans the first as a draft the session list hides.
    const create = vi.fn().mockResolvedValue(fakeSession("new-3"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const first = await result.current();
    const again = await result.current();

    expect(again).toBe(first);
    expect(create).toHaveBeenCalledTimes(1);
  });

  it("still creates when the fresh session is not a draft, or a cwd is asked for", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-4"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await result.current();
    // A message-less session that is NOT a draft may simply not have loaded its
    // history yet — reuse would drop the user back into a conversation.
    useAgentSessionStore.setState({ draftSessionIds: new Set<string>() });
    await result.current();
    expect(create).toHaveBeenCalledTimes(2);

    useAgentSessionStore.getState().markDraft("new-4");
    await result.current({ cwd: "/tmp/other" });
    expect(create).toHaveBeenCalledTimes(3);
  });

  it("only joins an in-flight create that asked for the same thing", async () => {
    // Joining any create in flight handed the caller someone else's session: a
    // project "+" landed in the runtime's default directory, and a welcome-composer
    // send got a session its typed message was never queued against — chatSend
    // fires this and never inspects the id, so the text was simply gone.
    let release: ((session: unknown) => void) | undefined;
    const create = vi
      .fn()
      .mockImplementationOnce(() => new Promise((resolve) => (release = resolve)))
      .mockResolvedValue(fakeSession("second"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const bare = result.current();
    const withInput = result.current({ firstInput: agentTextInput("don't lose me") });
    release?.(fakeSession("first"));

    expect(await bare).toBe("first");
    expect(await withInput).toBe("second");
    expect(create).toHaveBeenCalledTimes(2);
    expect(useAgentSessionStore.getState().takePendingMessage("second")).toEqual({
      input: agentTextInput("don't lose me"),
      runOptions: {},
    });
  });

  it("returns null + doesn't throw when create fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    stubCreate(vi.fn().mockRejectedValue(new Error("boom")));
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current({ firstInput: agentTextInput("x") })).resolves.toBeNull();
    expect(useAgentSessionStore.getState().activeSessionId).toBe("");
  });

  it("re-entrant calls join the in-flight create (double-click ≠ two sessions)", async () => {
    // sessions.create is a round-trip; a second "New" click inside that
    // window must not create a second backend session + open-session entry.
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
    expect(useAgentSessionStore.getState().openSessionIds).toEqual(["new-3"]);
  });
});
