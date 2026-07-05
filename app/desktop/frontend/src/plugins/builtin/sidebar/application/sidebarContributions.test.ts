import { describe, expect, it } from "vitest";
import { sidebarFooterSlot } from "./sidebarContributions";

function Component() {
  return null;
}

describe("sidebarFooterSlot", () => {
  it("projects the footer component into the sidebar footer slot spec", () => {
    expect(sidebarFooterSlot(Component)).toEqual({
      id: "user-card",
      order: 0,
      component: Component,
    });
  });
});
