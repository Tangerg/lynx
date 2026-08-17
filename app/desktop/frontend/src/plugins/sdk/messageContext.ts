// MessageContext — exposes the currently-rendering Message and its owning
// Session to plugin components mounted inside a per-message Slot.
//
// Defined as its own module so plugin SDK consumers can import the hook
// without dragging in the React component tree of `MessageBlock`.

import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { createContext, use } from "react";

export interface MessageContextValue {
  sessionId: string;
  message: Message;
}

export const MessageContext = createContext<MessageContextValue | null>(null);

/**
 * Read the message a plugin's `message.*` slot component is rendering inside.
 * Throws if used outside a MessageBlock — that's almost certainly a
 * plugin-author bug.
 */
export function useCurrentMessage(): Message {
  const ctx = use(MessageContext);
  if (!ctx) throw new Error("useCurrentMessage() must be called inside a MessageBlock");
  return ctx.message;
}

/** Read the exact Session whose transcript owns the current message. */
export function useCurrentMessageSessionId(): string {
  const ctx = use(MessageContext);
  if (!ctx) throw new Error("useCurrentMessageSessionId() must be called inside a MessageBlock");
  return ctx.sessionId;
}
