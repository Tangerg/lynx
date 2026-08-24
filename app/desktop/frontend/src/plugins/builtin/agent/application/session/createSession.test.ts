// Session creation is an exact-workspace command. The hook is used by project
// selectors; the imperative facade inherits cwd only from the active Session.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { navigator } from "@/lib/navigation";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient, Methods } from "@/rpc";
import { asSessionId } from "@/rpc";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import { installAgentRuntimeGateway } from "@/plugins/builtin/agent/adapters/agentRuntimeGateway";
import { createSession, type CreateSessionOptions, useCreateSession } from "./createSession";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "./sessionQueries";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

function stubCreate(create: Methods["sessions"]["create"]) {
  setContainer({ client: () => ({ sessions: { create } }) as unknown as LyraClient });
}

afterEach(() => {
  resetContainer();
  queryClient.clear();
  navigator().go({ session: "" });
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
    draftSessionIds: new Set<string>(),
    freshDraftSessionIds: new Set<string>(),
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

function summary(id: string, cwd: string): AgentSessionSummary {
  return {
    id,
    revision: 1,
    title: "Current",
    status: "idle",
    provider: "openai",
    model: "gpt-5",
    cwd,
    time: "2026-08-20T00:00:00Z",
  };
}

describe("useCreateSession", () => {
  it("creates a hidden draft in the chosen exact directory and opens it", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-cwd"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const id = await result.current({ cwd: "/tmp/proj" });

    expect(id).toBe("new-cwd");
    expect(create).toHaveBeenCalledWith(
      { workspace: { path: "/tmp/proj" } },
      expect.any(AbortSignal),
    );
    const state = useAgentSessionStore.getState();
    expect(navigator().get().session).toBe("new-cwd");
    expect(state.openSessionIds).toContain("new-cwd");
    expect(state.draftSessionIds.has("new-cwd")).toBe(true);
  });

  it("never delegates an empty working directory to the Runtime default", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("implicit-home"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current({ cwd: "" })).resolves.toBeNull();

    expect(create).not.toHaveBeenCalled();
    expect(navigator().get().session).toBe("");
  });

  it("reuses the active fresh draft only when New proves the same project destination", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("new-project"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });
    const destination = {
      cwd: "/tmp/current-project",
      reuseFreshDraft: true,
    } satisfies CreateSessionOptions;

    const first = await result.current(destination);
    const again = await result.current(destination);

    expect(again).toBe(first);
    expect(create).toHaveBeenCalledTimes(1);
  });

  it("does not reuse an ordinary message-less Session or an explicit project selection", async () => {
    const create = vi
      .fn()
      .mockResolvedValueOnce(fakeSession("new-1"))
      .mockResolvedValueOnce(fakeSession("new-2"))
      .mockResolvedValueOnce(fakeSession("new-3"));
    stubCreate(create);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await result.current({ cwd: "/tmp/current", reuseFreshDraft: true });
    useAgentSessionStore.setState({
      draftSessionIds: new Set<string>(),
      freshDraftSessionIds: new Set<string>(),
    });
    await result.current({ cwd: "/tmp/current", reuseFreshDraft: true });
    useAgentSessionStore.getState().markDraft("new-2");
    await result.current({ cwd: "/tmp/other" });

    expect(create).toHaveBeenCalledTimes(3);
  });

  it("joins only an in-flight create for the same exact cwd", async () => {
    let release: ((session: ReturnType<typeof fakeSession>) => void) | undefined;
    const create = vi
      .fn()
      .mockImplementationOnce(
        () => new Promise<ReturnType<typeof fakeSession>>((resolve) => (release = resolve)),
      )
      .mockResolvedValue(fakeSession("second"));
    stubCreate(create as unknown as Methods["sessions"]["create"]);
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    const first = result.current({ cwd: "/tmp/a" });
    const joined = result.current({ cwd: "/tmp/a" });
    const distinct = result.current({ cwd: "/tmp/b" });
    release?.(fakeSession("first"));

    expect(await first).toBe("first");
    expect(await joined).toBe("first");
    expect(await distinct).toBe("second");
    expect(create).toHaveBeenCalledTimes(2);
  });

  it("returns null without moving selection when create fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    stubCreate(vi.fn().mockRejectedValue(new Error("boom")));
    const { result } = renderHook(() => useCreateSession(), { wrapper });

    await expect(result.current({ cwd: "/tmp/project" })).resolves.toBeNull();
    expect(navigator().get().session).toBe("");
  });

  it("does not join or publish a create owned by a replaced Plugin Host", async () => {
    let releaseRetired!: (value: ReturnType<typeof fakeSession>) => void;
    const retiredCreate = vi.fn(
      () =>
        new Promise<ReturnType<typeof fakeSession>>((resolve) => {
          releaseRetired = resolve;
        }),
    );
    stubCreate(retiredCreate as unknown as Methods["sessions"]["create"]);
    const { result } = renderHook(() => useCreateSession(), { wrapper });
    const retired = result.current({ cwd: "/tmp/retired" });

    const successorCreate = vi.fn().mockResolvedValue(fakeSession("successor"));
    stubCreate(successorCreate);
    const disposeSuccessor = installAgentRuntimeGateway();
    const successor = result.current({ cwd: "/tmp/successor" });

    await Promise.resolve();
    const successorStartedBeforeRetiredSettlement = successorCreate.mock.calls.length;
    let retiredSettled = false;
    void retired.then(() => {
      retiredSettled = true;
    });
    await flushMicrotasks();
    const retiredSettledBeforeOldRPC = retiredSettled;
    releaseRetired(fakeSession("retired"));
    try {
      await expect(retired).resolves.toBeNull();
      await expect(successor).resolves.toBe("successor");
      expect(successorStartedBeforeRetiredSettlement).toBe(1);
      expect(retiredSettledBeforeOldRPC).toBe(true);
      expect(navigator().get().session).toBe("successor");
    } finally {
      disposeSuccessor.dispose();
    }
  });
});

describe("imperative New", () => {
  it("inherits the exact active Session cwd", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("next"));
    stubCreate(create);
    navigator().go({ session: "current" });
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [summary("current", "/tmp/current")]);

    await expect(createSession()).resolves.toBe("next");

    expect(create).toHaveBeenCalledWith(
      { workspace: { path: "/tmp/current" } },
      expect.any(AbortSignal),
    );
  });

  it("does not mutate when no active Session or authoritative cwd exists", async () => {
    const create = vi.fn().mockResolvedValue(fakeSession("implicit-home"));
    stubCreate(create);

    await expect(createSession()).resolves.toBeNull();
    navigator().go({ session: "unresolved" });
    await expect(createSession()).resolves.toBeNull();

    expect(create).not.toHaveBeenCalled();
  });
});

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}
