import type { ComposerKeyBindingSpec } from "@/plugins/sdk";

export type ComposerKeyHandler = ComposerKeyBindingSpec["handler"];

export interface ComposerKeyHandlers {
  send: ComposerKeyHandler;
  approveOrSend: ComposerKeyHandler;
  declineApproval: ComposerKeyHandler;
  stopRun: ComposerKeyHandler;
  historyPrevious: ComposerKeyHandler;
  historyNext: ComposerKeyHandler;
}

export function composerKeyBindings(handlers: ComposerKeyHandlers): ComposerKeyBindingSpec[] {
  return [
    { key: "Enter", description: "composer.key.sendDesc", handler: handlers.send },
    {
      key: "Mod+Enter",
      description: "composer.key.approveDesc",
      handler: handlers.approveOrSend,
    },
    {
      key: "Mod+Shift+Backspace",
      description: "composer.key.declineDesc",
      handler: handlers.declineApproval,
    },
    { key: "Escape", description: "composer.key.stopDesc", handler: handlers.stopRun },
    {
      key: "ArrowUp",
      description: "composer.key.historyPrevDesc",
      handler: handlers.historyPrevious,
    },
    {
      key: "ArrowDown",
      description: "composer.key.historyNextDesc",
      handler: handlers.historyNext,
    },
  ];
}
