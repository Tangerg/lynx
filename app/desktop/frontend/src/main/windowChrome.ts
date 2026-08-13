// The window's own controls are the platform's, and on macOS they sit over the app's
// content. This is where the app learns their geometry instead of assuming it.
//
// Two numbers, because two things depend on them. The gutter is where the cluster
// ends, so the header's first control clears it. The centre line is what that control
// centres ON: the header's own centre is a pixel away from the marks', and aligning to
// the wrong one is exactly the 5px error the hand-drawn controls carried (they were
// measured against a titlebar this window does not have).
//
// Applied before the first render (main.tsx awaits it) so the header is never laid out
// against one number and then moved, and re-read on resize because the titlebar is
// rebuilt entering and leaving fullscreen — where the marks are gone entirely and the
// gutter has to collapse.

import { getContainer } from "@/main/container";

/** Between the last mark and whatever the header puts next to it. */
const CONTROL_GAP_PX = 6;

const GUTTER_PROPERTY = "--window-controls-gutter";
const CENTRE_PROPERTY = "--window-controls-centre";

export async function applyWindowChrome(): Promise<void> {
  const chrome = await getContainer().desktop.windowChrome();
  const root = document.documentElement;
  // Nothing to measure: clear the overrides rather than write a guess, so a browser
  // tab and a visual fixture render the stylesheet's declared geometry instead of a
  // fallback this module invented.
  if (!chrome) {
    root.style.removeProperty(GUTTER_PROPERTY);
    root.style.removeProperty(CENTRE_PROPERTY);
    return;
  }
  const hidden = chrome.controlsInlineEnd === 0;
  root.style.setProperty(
    GUTTER_PROPERTY,
    hidden ? "0px" : `${chrome.controlsInlineEnd + CONTROL_GAP_PX}px`,
  );
  root.style.setProperty(CENTRE_PROPERTY, hidden ? "" : `${chrome.controlsCentreY}px`);
  if (hidden) root.style.removeProperty(CENTRE_PROPERTY);
}

/** Keeps the geometry current for as long as the window lives. Resize is the one event
 *  every fullscreen transition and display change has in common. */
export function watchWindowChrome(): () => void {
  const refresh = () => void applyWindowChrome();
  addEventListener("resize", refresh);
  return () => removeEventListener("resize", refresh);
}
