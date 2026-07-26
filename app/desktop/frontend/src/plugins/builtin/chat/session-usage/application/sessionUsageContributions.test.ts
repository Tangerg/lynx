import { describe, expect, it } from "vitest";
import { sessionUsageStatusSlot } from "./sessionUsageContributions";

function Component() {
  return null;
}

describe("sessionUsageStatusSlot", () => {
  it("projects the usage component into the chat banner slot spec", () => {
    expect(sessionUsageStatusSlot(Component)).toEqual({
      id: "session-usage",
      order: 10,
      component: Component,
    });
  });
});
