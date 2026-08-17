import { describe, expect, it } from "vitest";
import { messageActionsVisibility } from "./actionBarVisibility";

describe("messageActionsVisibility", () => {
  it("does not materialize actions for an unfinished tail when root attention briefly settles", () => {
    expect(
      messageActionsVisibility({
        materialization: "active",
        isRunning: false,
        isLast: true,
      }),
    ).toBe("absent");
  });

  it("hides every message's actions while a run streams", () => {
    expect(
      messageActionsVisibility({ materialization: "settled", isRunning: true, isLast: true }),
    ).toBe("hidden");
    expect(
      messageActionsVisibility({ materialization: "settled", isRunning: true, isLast: false }),
    ).toBe("hidden");
  });

  it("pins the last settled message's actions open", () => {
    expect(
      messageActionsVisibility({ materialization: "settled", isRunning: false, isLast: true }),
    ).toBe("pinned");
  });

  it("reveals earlier settled messages on hover", () => {
    expect(
      messageActionsVisibility({ materialization: "settled", isRunning: false, isLast: false }),
    ).toBe("hover");
  });
});
