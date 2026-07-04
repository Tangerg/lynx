import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useProbe } from "./useProbe";

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

describe("useProbe", () => {
  it("run -> ok reflects success", async () => {
    const { result } = renderHook(() => useProbe());
    await act(async () => {
      await result.current.run(async () => ({ ok: true }), "fallback");
    });
    expect(result.current.probe).toEqual({ state: "ok" });
  });

  it("run -> not-ok surfaces the result error, falling back when absent", async () => {
    const { result } = renderHook(() => useProbe());
    await act(async () => {
      await result.current.run(async () => ({ ok: false, error: "bad key" }), "fallback");
    });
    expect(result.current.probe).toEqual({ state: "error", reason: "bad key" });
    await act(async () => {
      await result.current.run(async () => ({ ok: false }), "fallback");
    });
    expect(result.current.probe).toEqual({ state: "error", reason: "fallback" });
  });

  it("run -> thrown Error surfaces its message", async () => {
    const { result } = renderHook(() => useProbe());
    await act(async () => {
      await result.current.run(async () => {
        throw new Error("network down");
      }, "fallback");
    });
    expect(result.current.probe).toEqual({ state: "error", reason: "network down" });
  });

  it("drops a superseded run's result", async () => {
    const { result } = renderHook(() => useProbe());
    const slow = deferred<{ ok: boolean; error?: string }>();
    let firstDone!: Promise<void>;
    act(() => {
      firstDone = result.current.run(() => slow.promise, "fallback");
    });
    expect(result.current.probe).toEqual({ state: "busy" });
    await act(async () => {
      await result.current.run(async () => ({ ok: true }), "fallback");
    });
    expect(result.current.probe).toEqual({ state: "ok" });
    await act(async () => {
      slow.resolve({ ok: false, error: "stale" });
      await firstDone;
    });
    expect(result.current.probe).toEqual({ state: "ok" });
  });

  it("reset invalidates an in-flight run and returns to idle", async () => {
    const { result } = renderHook(() => useProbe());
    const slow = deferred<{ ok: boolean; error?: string }>();
    let done!: Promise<void>;
    act(() => {
      done = result.current.run(() => slow.promise, "fallback");
    });
    act(() => result.current.reset());
    expect(result.current.probe).toEqual({ state: "idle" });
    await act(async () => {
      slow.resolve({ ok: true });
      await done;
    });
    expect(result.current.probe).toEqual({ state: "idle" });
  });
});
