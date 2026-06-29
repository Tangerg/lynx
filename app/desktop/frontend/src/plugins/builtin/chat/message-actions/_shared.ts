// Shared Tailwind chrome for the per-message action button plugins
// (copy / edit / regenerate / feedback). Domain logic lives in
// `@/lib/agent/messageContent`.

/** Base Tailwind class string for the hover-revealed icon buttons in
 *  the message action bar. Rounding is applied per-role by each
 *  component (`rounded-full` for user, `rounded-md` for assistant). */
export const ACTION_BTN_BASE =
  "inline-flex h-7 w-7 items-center justify-center border-0 bg-transparent text-fg-faint transition-colors hover:bg-fg/[0.06] hover:text-fg";
