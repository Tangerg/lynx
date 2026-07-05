import { describe, expect, it } from "vitest";
import {
  composerApprovalSlot,
  composerAttachSlot,
  composerModelSlot,
  composerSendSlot,
} from "./composerContributions";

function Component() {
  return null;
}

describe("composer contributions", () => {
  it("projects toolbar components into ordered layout slot specs", () => {
    expect(composerAttachSlot(Component)).toEqual({
      id: "attach",
      order: 0,
      component: Component,
    });
    expect(composerApprovalSlot(Component)).toEqual({
      id: "approval",
      order: 1,
      component: Component,
    });
    expect(composerModelSlot(Component)).toEqual({
      id: "model",
      order: 2,
      component: Component,
    });
    expect(composerSendSlot(Component)).toEqual({
      id: "send",
      order: 100,
      component: Component,
    });
  });
});
