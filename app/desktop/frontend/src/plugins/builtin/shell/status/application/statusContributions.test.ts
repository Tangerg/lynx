import { describe, expect, it } from "vitest";
import { notificationsStatusSlot } from "./statusContributions";

function Component() {
  return null;
}

describe("notificationsStatusSlot", () => {
  it("projects the notifications component into the sidebar status slot spec", () => {
    expect(notificationsStatusSlot(Component)).toEqual({
      id: "notifications",
      order: 10,
      component: Component,
    });
  });
});
