import { describe, expect, it } from "vitest";
import { messageActionsVisibility } from "./actionBarVisibility";

describe("messageActionsVisibility", () => {
  it("hides every message's actions while a run streams", () => {
    expect(messageActionsVisibility({ isRunning: true, isLast: true })).toBe("hidden");
    expect(messageActionsVisibility({ isRunning: true, isLast: false })).toBe("hidden");
  });

  it("pins the last settled message's actions open", () => {
    expect(messageActionsVisibility({ isRunning: false, isLast: true })).toBe("pinned");
  });

  it("reveals earlier settled messages on hover", () => {
    expect(messageActionsVisibility({ isRunning: false, isLast: false })).toBe("hover");
  });
});
