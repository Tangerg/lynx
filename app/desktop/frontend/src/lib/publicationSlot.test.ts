import { describe, expect, it, vi } from "vitest";
import { createPublicationSlot } from "./publicationSlot";

describe("publication slot", () => {
  it("publishes the successor before retiring its predecessor", () => {
    const slot = createPublicationSlot<{ id: string }>();
    const first = { id: "first" };
    const second = { id: "second" };
    slot.publish(first, vi.fn());
    const retire = vi.fn((predecessor: { id: string }) => {
      expect(predecessor).toBe(first);
      expect(slot.current()).toBe(second);
      expect(slot.owns(first)).toBe(false);
      expect(slot.owns(second)).toBe(true);
    });

    slot.publish(second, retire);

    expect(retire).toHaveBeenCalledOnce();
  });

  it("does not let a stale or repeated withdrawal clear the successor", () => {
    const slot = createPublicationSlot<{ id: string }>();
    const first = { id: "first" };
    const second = { id: "second" };
    slot.publish(first, vi.fn());
    slot.publish(second, vi.fn());

    expect(slot.withdraw(first)).toBe(false);
    expect(slot.current()).toBe(second);
    expect(slot.withdraw(second)).toBe(true);
    expect(slot.withdraw(second)).toBe(false);
    expect(slot.current()).toBeNull();
  });
});
