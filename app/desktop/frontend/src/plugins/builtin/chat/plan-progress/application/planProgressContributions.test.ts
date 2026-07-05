import { describe, expect, it } from "vitest";
import { planProgressBannerSlot } from "./planProgressContributions";

function Component() {
  return null;
}

describe("planProgressBannerSlot", () => {
  it("projects the progress component into the chat banner slot spec", () => {
    expect(planProgressBannerSlot(Component)).toEqual({
      id: "plan-progress",
      order: 0,
      component: Component,
    });
  });
});
