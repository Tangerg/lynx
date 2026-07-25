import { configureFontAvailabilityPort } from "../application/ports/fontAvailability";

// `document.fonts.check()` reports whether a family is loaded and matchable at a
// given size. `queryLocalFonts()` would enumerate properly but is permission-
// gated and unsupported in WebKit (Wails ships WebKit on macOS), so a curated
// candidate list filtered through this is the portable answer.
//
// Missing API is treated as "available": on a runtime that cannot tell us, hiding
// every font would leave the picker empty, which is worse than offering one that
// silently falls back.
function isAvailable(family: string): boolean {
  if (typeof document === "undefined") return false;
  const check = document.fonts?.check;
  if (typeof check !== "function") return true;
  try {
    return document.fonts.check(`12px "${family}"`);
  } catch {
    return false;
  }
}

export function installBrowserFontAvailability(): () => void {
  return configureFontAvailabilityPort({ isAvailable });
}
