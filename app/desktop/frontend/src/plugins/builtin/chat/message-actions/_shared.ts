// Shared Tailwind chrome for the three message-action button plugins
// (copy / edit / regenerate). The flatten / clipboard helpers that used
// to live here moved to `@/lib/agent/messageContent` once a third consumer
// (the right-click message context menu) showed up — plugin-internal
// `_shared` was the wrong home for shared domain logic.

/** Shared Tailwind class string for the hover-revealed icon buttons in
 *  the message header (copy / edit / regenerate). */
export const ACTION_BTN_CLASSES =
  "inline-flex h-5 w-5 items-center justify-center rounded border-0 bg-transparent text-fg-faint cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg";
