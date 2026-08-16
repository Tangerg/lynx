import { describe, expect, it, vi } from "vitest";
import { onGoalLauncherRequest, requestGoalLauncher } from "./goalLauncherRequest";

describe("goal launcher request", () => {
  it("reports that nothing took a request when no launcher is listening", () => {
    expect(requestGoalLauncher("ship it")).toBe(false);
  });

  it("delivers the objective to a listening launcher", () => {
    const open = vi.fn();
    const stop = onGoalLauncherRequest(open);

    expect(requestGoalLauncher("ship the Linux gate")).toBe(true);
    expect(open).toHaveBeenCalledWith("ship the Linux gate");

    stop();
  });

  // The reason this is a signal and not stored state: a request made while no
  // launcher is mounted must be GONE, not waiting. Stored, it would spring the
  // dialog open the next time the launcher happened to appear.
  it("keeps nothing for a launcher that mounts later", () => {
    expect(requestGoalLauncher("stale")).toBe(false);

    const open = vi.fn();
    const stop = onGoalLauncherRequest(open);
    expect(open).not.toHaveBeenCalled();

    stop();
  });

  it("stops delivering once the launcher unsubscribes", () => {
    const open = vi.fn();
    onGoalLauncherRequest(open)();

    expect(requestGoalLauncher("gone")).toBe(false);
    expect(open).not.toHaveBeenCalled();
  });

  // A launcher that unsubscribes as it handles the request would otherwise mutate
  // the set being iterated.
  it("survives a listener that unsubscribes while handling", () => {
    const stop = onGoalLauncherRequest(() => stop());
    expect(() => requestGoalLauncher("reentrant")).not.toThrow();
    expect(requestGoalLauncher("after")).toBe(false);
  });
});
