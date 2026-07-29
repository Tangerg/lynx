// Window-shell geometry — the numbers the drawer is sized by.
//
// Lives here rather than beside the shell components because both the view layer
// (which clamps a live drag) and the preference store (which persists the
// settled width) need them, and `state` must not import `ui`.

export const SIDEBAR_MIN_WIDTH_PX = 208;
export const SIDEBAR_DEFAULT_WIDTH_PX = 256;

/**
 * How much room a dock view's material needs.
 *
 * `light` is a list or an inspector, read at a glance beside the conversation.
 * `review` is a diff: code in a column too narrow to hold a hunk is not a
 * review, it is a wrapping contest.
 *
 * The dock remembers ONE WIDTH PER DENSITY rather than one width full stop.
 * With a single width, dragging the dock wide enough to review a change left the
 * next thing opened there — a todo list, a run summary — at review width, and
 * the user had to drag it back every time they switched material.
 */
export type DockDensity = "light" | "review";

export const DOCK_MIN_WIDTH_PX = 300;
export const DOCK_DEFAULT_WIDTHS: Record<DockDensity, number> = {
  light: 420,
  // A diff plus its changed-file navigator. Narrower than this and the
  // navigator — `min(42%, 28rem)` of the panel — leaves the code column too
  // thin to read a hunk in without wrapping every line.
  review: 720,
};

/** Every density, derived from the widths so the record stays the only author. */
export const DOCK_DENSITIES = Object.keys(DOCK_DEFAULT_WIDTHS) as readonly DockDensity[];

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
