import { describe, expect, it, vi } from "vitest";
import { AgentCommandOwner } from "./agentCommandOwner";

describe("AgentCommandOwner", () => {
  it("retires predecessor effects before the successor accepts commands", () => {
    const rollback = vi.fn();
    const retired = AgentCommandOwner.install();
    retired.trackEffect(rollback);

    const successor = AgentCommandOwner.install();

    expect(rollback).toHaveBeenCalledOnce();
    expect(retired.isCurrent()).toBe(false);
    expect(successor.isCurrent()).toBe(true);
    successor.dispose();
  });

  it("does not let a stale disposer retire the successor", () => {
    const retired = AgentCommandOwner.install();
    const successor = AgentCommandOwner.install();

    retired.dispose();

    expect(successor.isCurrent()).toBe(true);
    expect(() => successor.assertCurrent()).not.toThrow();
    successor.dispose();
  });
});
