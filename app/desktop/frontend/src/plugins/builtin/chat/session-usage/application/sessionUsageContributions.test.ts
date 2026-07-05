import { describe, expect, it } from "vitest";
import { sessionUsageBannerSlot } from "./sessionUsageContributions";

function Component() {
  return null;
}

describe("sessionUsageBannerSlot", () => {
  it("projects the usage component into the chat banner slot spec", () => {
    expect(sessionUsageBannerSlot(Component)).toEqual({
      id: "session-usage",
      order: 10,
      component: Component,
    });
  });
});
