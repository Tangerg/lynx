// startTask lifecycle — ids are a supported cross-call handle
// (TaskStartOptions.id), so a restarted task reusing an id must be immune to
// the PREVIOUS generation: its settle's linger timer must not delete the new
// running entry, and the old handle's late settle/update must no-op.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { startTask, useTasksStore } from "./tasksStore";

const get = (id: string) => useTasksStore.getState().tasks.get(id);

beforeEach(() => {
  vi.useFakeTimers();
  useTasksStore.setState({ tasks: new Map() });
});
afterEach(() => vi.useRealTimers());

describe("startTask generation safety", () => {
  it("a settled task lingers then auto-removes", () => {
    const h = startTask("p", { id: "task:p:sync", label: "Sync" });
    h.succeed();
    expect(get("task:p:sync")?.status).toBe("succeeded");
    vi.runAllTimers();
    expect(get("task:p:sync")).toBeUndefined();
  });

  it("the previous settle's linger timer does not delete a restarted task", () => {
    const h1 = startTask("p", { id: "task:p:sync", label: "Sync" });
    h1.fail(new Error("boom")); // arms the linger timer
    vi.advanceTimersByTime(1000);

    // User retries — same id, fresh running entry — before the timer fires.
    vi.setSystemTime(Date.now() + 1); // distinct startedAt generation
    startTask("p", { id: "task:p:sync", label: "Sync" });
    vi.runAllTimers(); // the stale timer fires now

    expect(get("task:p:sync")?.status).toBe("running");
  });

  it("the previous generation's handle cannot settle the restarted task", () => {
    const h1 = startTask("p", { id: "task:p:sync", label: "Sync" });
    vi.setSystemTime(Date.now() + 1);
    startTask("p", { id: "task:p:sync", label: "Sync" }); // restart, new generation

    h1.fail(new Error("late failure from the old attempt"));
    expect(get("task:p:sync")?.status).toBe("running");

    h1.update({ message: "stale" });
    expect(get("task:p:sync")?.message).not.toBe("stale");
  });
});
