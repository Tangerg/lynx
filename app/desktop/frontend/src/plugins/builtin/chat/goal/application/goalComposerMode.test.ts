import { afterEach, describe, expect, it, vi } from "vitest";
import { GoalComposerModeOwner } from "./goalComposerMode";

let owner: GoalComposerModeOwner | null = null;

afterEach(() => {
  owner?.dispose();
  owner = null;
});

describe("GoalComposerModeOwner", () => {
  it("moves one Session from armed through authoritative start settlement", () => {
    owner = GoalComposerModeOwner.install();
    const changed = vi.fn();
    owner.subscribe(changed);

    expect(owner.activate("ses_a")).toBe(true);
    expect(owner.snapshot()).toMatchObject({ sessionId: "ses_a", phase: "armed" });
    expect(owner.begin("ses_a")).toBe(true);
    expect(owner.snapshot().phase).toBe("starting");
    expect(owner.finish("ses_a", true)).toBe(true);
    expect(owner.snapshot().phase).toBe("inactive");
    expect(changed).toHaveBeenCalledTimes(3);
  });

  it("keeps a failed authoritative start armed and preserves replacement intent until confirmed", () => {
    owner = GoalComposerModeOwner.install();
    const start = vi.fn();

    owner.activate("ses_a");
    expect(owner.requestReplacement("ses_a", "Old goal", start)).toBe(true);
    expect(owner.snapshot()).toMatchObject({
      sessionId: "ses_a",
      phase: "confirming",
      replacedObjective: "Old goal",
    });
    expect(owner.confirmReplacement("ses_a")).toBe(true);
    expect(start).toHaveBeenCalledOnce();
    expect(owner.finish("ses_a", false)).toBe(true);
    expect(owner.snapshot().phase).toBe("armed");
  });

  it("retires predecessor callbacks when a successor plugin generation installs", () => {
    owner = GoalComposerModeOwner.install();
    const stale = vi.fn();
    owner.activate("ses_a");
    owner.requestReplacement("ses_a", "Old goal", stale);

    const successor = GoalComposerModeOwner.install();

    expect(owner.confirmReplacement("ses_a")).toBe(false);
    expect(stale).not.toHaveBeenCalled();
    expect(GoalComposerModeOwner.current()).toBe(successor);
    successor.dispose();
  });
});
