// Window-shell geometry — the numbers the drawer is sized by.
//
// Lives here rather than beside the shell components because both the view layer
// (which clamps a live drag) and the preference store (which persists the
// settled width) need them, and `state` must not import `ui`.

export const SIDEBAR_MIN_WIDTH_PX = 208;
export const SIDEBAR_DEFAULT_WIDTH_PX = 256;

/** Floor for the reading column. Enforced against the live window width, so the
 *  drawer's maximum shrinks with the window instead of being a fixed number. */
const MAIN_MIN_WIDTH_PX = 640;

export function clampSidebarWidth(width: number, shellWidth: number): number {
  const max = Math.max(SIDEBAR_MIN_WIDTH_PX, shellWidth - MAIN_MIN_WIDTH_PX);
  return Math.round(Math.min(max, Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}
