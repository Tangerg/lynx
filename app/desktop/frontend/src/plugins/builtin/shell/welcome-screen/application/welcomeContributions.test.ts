import { describe, expect, it } from "vitest";
import { welcomeEmptySlot } from "./welcomeContributions";

function Component() {
  return null;
}

describe("welcomeEmptySlot", () => {
  it("projects the welcome component into the empty chat slot spec", () => {
    expect(welcomeEmptySlot(Component)).toEqual({
      id: "welcome",
      order: 0,
      component: Component,
    });
  });
});
