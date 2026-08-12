import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { useToggleFavorite } from "./favoriteSession";
import { useRenameSession } from "./renameSession";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "./sessionQueries";

let restoreRuntime: (() => void) | undefined;

function session(): AgentSessionSummary {
  return {
    id: "ses_deleted",
    revision: 3,
    title: "before",
    status: "idle",
    model: "gpt-5",
    cwd: "",
    time: "2026-08-12T00:00:00Z",
  };
}

afterEach(() => {
  restoreRuntime?.();
  restoreRuntime = undefined;
  vi.restoreAllMocks();
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("optimistic Session summary mutations", () => {
  it.each([
    {
      name: "rename",
      run: async () => {
        const { result } = renderHook(() => useRenameSession());
        await result.current("ses_deleted", 3, "after");
      },
    },
    {
      name: "favorite",
      run: async () => {
        const { result } = renderHook(() => useToggleFavorite());
        await result.current("ses_deleted", 3, true);
      },
    },
  ])("revalidates authoritative membership when $name loses a delete race", async ({ run }) => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession: vi.fn().mockRejectedValue(new Error("deleted concurrently")),
    } as unknown as AgentRuntimeGateway);

    await run();

    expect(invalidate).toHaveBeenCalledWith({ queryKey: [AGENT_SESSIONS_KEY] });
  });

  it("serializes local conditional writes and carries the committed revision", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const rename = deferred<{ revision: number }>();
    const favorite = deferred<{ revision: number }>();
    const updateSession = vi
      .fn()
      .mockImplementationOnce(() => rename.promise)
      .mockImplementationOnce(() => favorite.promise);
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const renameHook = renderHook(() => useRenameSession());
    const favoriteHook = renderHook(() => useToggleFavorite());

    let renaming!: Promise<void>;
    let favoriting!: Promise<void>;
    await act(async () => {
      renaming = renameHook.result.current("ses_deleted", 3, "after");
      favoriting = favoriteHook.result.current("ses_deleted", 3, true);
      await vi.waitFor(() => expect(updateSession).toHaveBeenCalledTimes(1));
    });
    expect(updateSession).toHaveBeenNthCalledWith(1, {
      sessionId: "ses_deleted",
      expectedRevision: 3,
      title: "after",
    });

    rename.resolve({ revision: 4 });
    await vi.waitFor(() => expect(updateSession).toHaveBeenCalledTimes(2));
    expect(updateSession).toHaveBeenNthCalledWith(2, {
      sessionId: "ses_deleted",
      expectedRevision: 4,
      favorite: true,
    });
    favorite.resolve({ revision: 5 });
    await act(async () => Promise.all([renaming, favoriting]));

    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      { ...session(), title: "after", favorite: true, revision: 5 },
    ]);
  });

  it("rolls back only its own field after a preceding local write commits", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const rename = deferred<{ revision: number }>();
    const updateSession = vi
      .fn()
      .mockImplementationOnce(() => rename.promise)
      .mockRejectedValueOnce(new Error("remote writer won"));
    restoreRuntime = configureAgentRuntimeGateway({
      updateSession,
    } as unknown as AgentRuntimeGateway);
    const renameHook = renderHook(() => useRenameSession());
    const favoriteHook = renderHook(() => useToggleFavorite());

    const renaming = renameHook.result.current("ses_deleted", 3, "after");
    const favoriting = favoriteHook.result.current("ses_deleted", 3, true);
    rename.resolve({ revision: 4 });
    await act(async () => Promise.all([renaming, favoriting]));

    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      { ...session(), title: "after", favorite: undefined, revision: 4 },
    ]);
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
