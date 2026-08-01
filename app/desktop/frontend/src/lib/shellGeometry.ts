// Window-shell geometry — the numbers the drawer is sized by.
//
// Lives here rather than beside the shell components because both the view layer
// (which clamps a live drag) and the preference store (which persists the
// settled width) need them, and `state` must not import `ui`.

export const SIDEBAR_MIN_WIDTH_PX = 208;
export const SIDEBAR_DEFAULT_WIDTH_PX = 256;

export const DOCK_MIN_WIDTH_PX = 300;
/** One stable workspace width. Switching tabs must not make both reading
 *  columns jump; the live row clamp still protects chat on narrow windows. */
export const DOCK_DEFAULT_WIDTH_PX = 520;

/** Floor for the reading column. Enforced against the live window width, so the
 *  drawer's maximum shrinks with the window instead of being a fixed number. */
const MAIN_MIN_WIDTH_PX = 640;

/** Floor for the chat column beside an open dock. Smaller than MAIN_MIN because
 *  the drawer can be collapsed to buy the room, whereas the dock is the thing
 *  the user just chose to look at. */
export const CHAT_MIN_WIDTH_PX = 420;

export function clampSidebarWidth(width: number, shellWidth: number): number {
  return Math.round(Math.min(maxSidebarWidth(shellWidth), Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}

/** Largest drawer width that still preserves the shell's reading column. */
export function maxSidebarWidth(shellWidth: number): number {
  return Math.max(SIDEBAR_MIN_WIDTH_PX, shellWidth - MAIN_MIN_WIDTH_PX);
}

/** Same shape as the drawer's clamp — the dock is a px-wide resizable column,
 *  not a fraction, so both edges of the card are sized by one mental model. */
export function clampDockWidth(width: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  return Math.round(Math.min(max, Math.max(DOCK_MIN_WIDTH_PX, width)));
}

/**
 * Largest dock width that preserves both halves of the split.
 *
 * The dock may never consume more than half the row, even when the conversation
 * floor would allow it. Keeping that rule here (rather than only in CSS) means
 * the rendered width, persisted value, pointer clamp, and ARIA range all speak
 * the same geometry.
 */
export function maxDockWidth(rowWidth: number): number {
  return Math.round(
    Math.max(DOCK_MIN_WIDTH_PX, Math.min(rowWidth / 2, rowWidth - CHAT_MIN_WIDTH_PX)),
  );
}
