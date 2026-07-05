import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { messageHasDraftContent } from "./messageActionContent";

export interface MessageContextMenuCopyState {
  canCopy: boolean;
  plain: string;
  code: string;
}

export interface MessageContextMenuModel {
  copyMarkdown: boolean;
  copyPlain: boolean;
  copyCode: boolean;
  user: {
    visible: boolean;
    editInComposer: boolean;
    editRerun: boolean;
    editRerunRestore: boolean;
    restore: boolean;
    restoreFiles: boolean;
    restoreBoth: boolean;
    fork: boolean;
  };
  assistant: {
    visible: boolean;
    regenerate: boolean;
    regenerateRestore: boolean;
  };
}

export function messageContextMenuModel({
  msg,
  copy,
  canRestoreFiles,
}: {
  msg: Message;
  copy: MessageContextMenuCopyState;
  canRestoreFiles: boolean;
}): MessageContextMenuModel {
  const isUser = msg.role === "user";
  const isAssistant = msg.role === "assistant";
  const hasRun = Boolean(msg.runId);
  const canEdit = isUser && messageHasDraftContent(msg);
  const canUseRunCheckpoint = isUser && hasRun;
  const canRegenerate = isAssistant;

  return {
    copyMarkdown: copy.canCopy,
    copyPlain: Boolean(copy.plain),
    copyCode: Boolean(copy.code),
    user: {
      visible: canEdit || canUseRunCheckpoint,
      editInComposer: canEdit,
      editRerun: canEdit && hasRun,
      editRerunRestore: canEdit && hasRun && canRestoreFiles,
      restore: canUseRunCheckpoint,
      restoreFiles: canUseRunCheckpoint && canRestoreFiles,
      restoreBoth: canUseRunCheckpoint && canRestoreFiles,
      fork: canUseRunCheckpoint,
    },
    assistant: {
      visible: canRegenerate,
      regenerate: canRegenerate,
      regenerateRestore: canRegenerate && canRestoreFiles,
    },
  };
}
