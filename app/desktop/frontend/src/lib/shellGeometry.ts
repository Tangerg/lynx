// Window-shell geometry — the numbers the drawer is sized by.
//
// Lives here rather than beside the shell components because both the view layer
// (which clamps a live drag) and the preference store (which persists the
// settled width) need them, and `state` must not import `ui`.

export const SIDEBAR_MIN_WIDTH_PX = 240;
export const SIDEBAR_DEFAULT_WIDTH_PX = 275;
const SIDEBAR_MAX_WIDTH_PX = 520;
const SIDEBAR_READING_MIN_WIDTH_PX = 240;

export const DOCK_MIN_WIDTH_PX = 420;
/** One stable workspace width. Switching tabs must not make both reading
 *  columns jump; the live row clamp still protects chat on narrow windows.
 *
 *  Sized for what this flank actually holds. 336 came from outline panels (Nova
 *  336, Zed 320, JetBrains 300) — navigators, where the content is a list of
 *  names. This one hosts diffs, terminals and file viewers, and the two shipping
 *  agent desktops that host the same things both land on 640 (Codex's right
 *  panel, MiniMax's file panel), with Codex refusing to go under 500 at all. At
 *  336 a unified diff had ~22 characters of code per line after its gutters and
 *  a navigator beside it, which is a column that renders but cannot be read.
 *
 *  On a narrow window `maxDockWidth` still takes this down to half the row, so
 *  the number is a ceiling the user drags away from rather than a promise. */
export const DOCK_DEFAULT_WIDTH_PX = 640;

/** Floor for the chat column beside an open dock. The transcript, composer and
 *  blocking HITL controls use the same readable minimum as the main plane. */
export const CHAT_MIN_WIDTH_PX = 640;

/** Whether both real columns can coexist. Below this boundary the dock must
 *  fold through its existing navigation owner instead of compressing either
 *  column below its operable minimum. */
export function canPresentDock(rowWidth: number): boolean {
  return rowWidth >= CHAT_MIN_WIDTH_PX + DOCK_MIN_WIDTH_PX;
}

export function clampSidebarWidth(width: number, shellWidth: number): number {
  return Math.round(Math.min(maxSidebarWidth(shellWidth), Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}

/** The Work Index has its own bounded source-list measure; it does not borrow
 *  the Context Dock's reading floor. The live clamp still leaves one
 *  minimum-width reading plane beside it. */
export function maxSidebarWidth(shellWidth: number): number {
  return Math.max(
    SIDEBAR_MIN_WIDTH_PX,
    Math.min(SIDEBAR_MAX_WIDTH_PX, shellWidth - SIDEBAR_READING_MIN_WIDTH_PX),
  );
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
