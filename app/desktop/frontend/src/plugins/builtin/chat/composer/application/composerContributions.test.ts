import { describe, expect, it } from "vitest";
import {
  composerApprovalSlot,
  composerAttachSlot,
  composerKeyBindings,
  composerModelSlot,
  composerModelRunOptions,
  composerPlaceholderSpecs,
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
    const bindings = composerKeyBindings((key) => `t:${key}`, {
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
      "t:composer.key.sendDesc",
      "t:composer.key.approveDesc",
      "t:composer.key.declineDesc",
      "t:composer.key.stopDesc",
      "t:composer.key.historyPrevDesc",
      "t:composer.key.historyNextDesc",
    ]);
  });

  it("projects built-in placeholder prompts into stable placeholder specs", () => {
    expect(composerPlaceholderSpecs()).toEqual([
      { id: "ask", text: "composer.placeholder.fallback" },
      { id: "debug", text: "composer.placeholder.debug" },
      { id: "implement", text: "composer.placeholder.implement" },
      { id: "refactor", text: "composer.placeholder.refactor" },
    ]);
  });

  it("projects the selected model resolver into the composer run options provider", () => {
    const options = composerModelRunOptions(() => ({ provider: "openai", model: "gpt" }));

    expect(options.id).toBe("composer.model");
    expect(options.priority).toBe(0);
    expect(options.resolve()).toEqual({ provider: "openai", model: "gpt" });
  });
});
