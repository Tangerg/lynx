import { describe, expect, it } from "vitest";
import {
  composerApprovalSlot,
  composerAttachSlot,
  composerKeyBindings,
  composerModelSlot,
  composerModelRunOptions,
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

  it("projects key binding handlers into stable composer key binding specs", () => {
    const handler = () => true;
    const bindings = composerKeyBindings({
      send: handler,
      approveOrSend: handler,
      declineApproval: handler,
      stopRun: handler,
      historyPrevious: handler,
      historyNext: handler,
    });

    expect(bindings.map((binding) => binding.key)).toEqual([
      "Enter",
      "Mod+Enter",
      "Mod+Shift+Backspace",
      "Escape",
      "ArrowUp",
      "ArrowDown",
    ]);
    expect(bindings.map((binding) => binding.description)).toEqual([
      "composer.key.sendDesc",
      "composer.key.approveDesc",
      "composer.key.declineDesc",
      "composer.key.stopDesc",
      "composer.key.historyPrevDesc",
      "composer.key.historyNextDesc",
    ]);
  });

  it("projects the selected model resolver into the composer run options provider", () => {
    const options = composerModelRunOptions(() => ({ provider: "openai", model: "gpt" }));

    expect(options.id).toBe("composer.model");
    expect(options.priority).toBe(0);
    expect(options.resolve()).toEqual({ provider: "openai", model: "gpt" });
  });
});
