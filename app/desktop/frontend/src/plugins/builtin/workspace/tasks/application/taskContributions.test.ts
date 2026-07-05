import { describe, expect, it } from "vitest";
import { tasksStatusSlot } from "./taskContributions";

function Component() {
  return null;
}

describe("tasksStatusSlot", () => {
  it("projects the tasks component into the sidebar status slot spec", () => {
    expect(tasksStatusSlot(Component)).toEqual({
      id: "tasks",
      order: 0,
      component: Component,
    });
  });
});
