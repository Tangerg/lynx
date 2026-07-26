// Window-shell geometry — the numbers the drawer is sized by.
//
// Lives here rather than beside the shell components because both the view layer
// (which clamps a live drag) and the preference store (which persists the
// settled width) need them, and `state` must not import `ui`.

export const SIDEBAR_MIN_WIDTH_PX = 208;
export const SIDEBAR_DEFAULT_WIDTH_PX = 256;

export const DOCK_MIN_WIDTH_PX = 300;
export const DOCK_DEFAULT_WIDTH_PX = 420;

/** Floor for the reading column. Enforced against the live window width, so the
 *  drawer's maximum shrinks with the window instead of being a fixed number. */
const MAIN_MIN_WIDTH_PX = 640;

/** Floor for the chat column beside an open dock. Smaller than MAIN_MIN because
 *  the drawer can be collapsed to buy the room, whereas the dock is the thing
 *  the user just chose to look at. */
const CHAT_MIN_WIDTH_PX = 420;

export function clampSidebarWidth(width: number, shellWidth: number): number {
  const max = Math.max(SIDEBAR_MIN_WIDTH_PX, shellWidth - MAIN_MIN_WIDTH_PX);
  return Math.round(Math.min(max, Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}

/** Same shape as the drawer's clamp — the dock is a px-wide resizable column,
 *  not a fraction, so both edges of the card are sized by one mental model. */
export function clampDockWidth(width: number, rowWidth: number): number {
  const max = Math.max(DOCK_MIN_WIDTH_PX, rowWidth - CHAT_MIN_WIDTH_PX);
  return Math.round(Math.min(max, Math.max(DOCK_MIN_WIDTH_PX, width)));
}
