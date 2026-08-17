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

  it("settles admitted work immediately when its generation retires", async () => {
    const retired = AgentCommandOwner.install();
    let settleOperation!: () => void;
    const pending = new Promise<void>((resolve) => {
      settleOperation = resolve;
    });
    const operation = retired.settle(pending);

    const successor = AgentCommandOwner.install();

    await expect(operation).rejects.toMatchObject({ message: "agent_command_owner_retired" });
    settleOperation();
    await pending;
    successor.dispose();
  });
});
