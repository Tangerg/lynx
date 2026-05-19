// MessageContext — exposes the currently-rendering Message to plugin
// components mounted inside a per-message Slot.
//
// Defined as its own module so plugin SDK consumers can import the hook
// without dragging in the React component tree of `MessageBlock`.

import { createContext, useContext } from "react";
import type { Message } from "@/protocol/agui/viewState";

export const MessageContext = createContext<Message | null>(null);

/**
 * Read the message a plugin's `message.*` slot component is rendering
 * inside of. Throws if used outside a MessageBlock — that's almost
 * certainly a plugin-author bug.
 */
export function useCurrentMessage(): Message {
  const ctx = useContext(MessageContext);
  if (!ctx) throw new Error("useCurrentMessage() must be called inside a MessageBlock");
  return ctx;
}
