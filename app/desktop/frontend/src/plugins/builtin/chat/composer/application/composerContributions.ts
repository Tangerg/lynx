import type {
  AgentRunOptionsProviderSpec,
  ComposerKeyBindingSpec,
  ComposerPlaceholderSpec,
  LayoutSlotSpec,
} from "@/plugins/sdk";

export type Translate = (key: string) => string;
export type ComposerKeyHandler = ComposerKeyBindingSpec["handler"];
export type RunOptionsResolver = AgentRunOptionsProviderSpec["resolve"];

export interface ComposerKeyHandlers {
  send: ComposerKeyHandler;
  approveOrSend: ComposerKeyHandler;
  declineApproval: ComposerKeyHandler;
  stopRun: ComposerKeyHandler;
  historyPrevious: ComposerKeyHandler;
  historyNext: ComposerKeyHandler;
}

export function composerAttachSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "attach",
    order: 0,
    component,
  };
}

export function composerApprovalSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "approval",
    order: 1,
    component,
  };
}

export function composerModelSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "model",
    order: 2,
    component,
  };
}

export function composerSendSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "send",
    order: 100,
    component,
  };
}

export function composerKeyBindings(
  t: Translate,
  handlers: ComposerKeyHandlers,
): ComposerKeyBindingSpec[] {
  return [
    {
      key: "Enter",
      description: t("composer.key.sendDesc"),
      handler: handlers.send,
    },
    {
      key: "Mod+Enter",
      description: t("composer.key.approveDesc"),
      handler: handlers.approveOrSend,
    },
    {
      key: "Mod+Shift+Backspace",
      description: t("composer.key.declineDesc"),
      handler: handlers.declineApproval,
    },
    {
      key: "Escape",
      description: t("composer.key.stopDesc"),
      handler: handlers.stopRun,
    },
    {
      key: "ArrowUp",
      description: t("composer.key.historyPrevDesc"),
      handler: handlers.historyPrevious,
    },
    {
      key: "ArrowDown",
      description: t("composer.key.historyNextDesc"),
      handler: handlers.historyNext,
    },
  ];
}

export function composerPlaceholderSpecs(): ComposerPlaceholderSpec[] {
  return [
    { id: "ask", text: "composer.placeholder.fallback" },
    { id: "debug", text: "composer.placeholder.debug" },
    { id: "implement", text: "composer.placeholder.implement" },
    { id: "refactor", text: "composer.placeholder.refactor" },
  ];
}

export function composerModelRunOptions(resolve: RunOptionsResolver): AgentRunOptionsProviderSpec {
  return {
    id: "composer.model",
    priority: 0,
    resolve,
  };
}
