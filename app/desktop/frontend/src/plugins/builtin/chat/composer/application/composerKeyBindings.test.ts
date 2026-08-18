import { describe, expect, it } from "vitest";
import { composerKeyBindings } from "./composerKeyBindings";

describe("composer key bindings", () => {
  it("maps each semantic action to its stable key and description", () => {
    const handler = () => true;
    const bindings = composerKeyBindings({
      send: handler,
      approveOrSend: handler,
      declineApproval: handler,
      stopRun: handler,
      historyPrevious: handler,
      historyNext: handler,
    });

    expect(bindings.map(({ key, description }) => [key, description])).toEqual([
      ["Enter", "composer.key.sendDesc"],
      ["Mod+Enter", "composer.key.approveDesc"],
      ["Mod+Shift+Backspace", "composer.key.declineDesc"],
      ["Escape", "composer.key.stopDesc"],
      ["ArrowUp", "composer.key.historyPrevDesc"],
      ["ArrowDown", "composer.key.historyNextDesc"],
    ]);
  });
});
