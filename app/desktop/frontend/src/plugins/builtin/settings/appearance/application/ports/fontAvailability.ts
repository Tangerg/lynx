import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * "Is this family installed?" — asked of whatever can answer it.
 *
 * A port because the candidate list is ours (a curated cross-platform set) while
 * the probe is the browser's: `document.fonts.check()`, chosen because
 * `queryLocalFonts()` is permission-gated and absent in WebKit. Keeping the probe
 * behind this port leaves the curated application list testable without a DOM.
 */
export interface FontAvailabilityPort {
  isAvailable(family: string): boolean;
}

const port = createSingletonPort<FontAvailabilityPort>("Font availability port is not configured");

export const configureFontAvailabilityPort = port.configure;
export const fontAvailability = port.get;
