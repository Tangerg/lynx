import { describe, expect, it } from "vitest";
import { toasterOverlaySlot } from "./toasterContributions";

function Component() {
  return null;
}

describe("toasterOverlaySlot", () => {
  it("projects the toaster component into the overlay slot spec", () => {
    expect(toasterOverlaySlot(Component)).toEqual({
      id: "toaster",
      order: 100,
      component: Component,
    });
  });
});
