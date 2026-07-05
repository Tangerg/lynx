import { describe, expect, it } from "vitest";
import {
  messageCopyActionSlot,
  messageEditActionSlot,
  messageFeedbackActionSlot,
  messageRegenerateActionSlot,
} from "./messageActionContributions";

function Component() {
  return null;
}

describe("message action contributions", () => {
  it("projects message action components into ordered layout slot specs", () => {
    expect(messageCopyActionSlot(Component)).toEqual({
      id: "copy",
      order: 0,
      component: Component,
    });
    expect(messageEditActionSlot(Component)).toEqual({
      id: "edit",
      order: 5,
      component: Component,
    });
    expect(messageRegenerateActionSlot(Component)).toEqual({
      id: "regenerate",
      order: 10,
      component: Component,
    });
    expect(messageFeedbackActionSlot(Component)).toEqual({
      id: "feedback",
      order: 15,
      component: Component,
    });
  });
});
