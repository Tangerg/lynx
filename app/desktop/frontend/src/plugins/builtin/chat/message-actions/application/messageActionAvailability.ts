import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { messageHasDraftContent } from "./messageActionContent";

export function canCopyMessage(copy: { canCopy: boolean }): boolean {
  return copy.canCopy;
}

export function canEditMessage(message: Message): boolean {
  return message.role === "user" && messageHasDraftContent(message);
}

export function canUseMessageRunCheckpoint(message: Message): boolean {
  return message.role === "user" && Boolean(message.runId);
}

export function canRegenerateMessage(message: Message): boolean {
  return message.role === "assistant";
}

export function canRateMessage(message: Message): boolean {
  return message.role === "assistant";
}
