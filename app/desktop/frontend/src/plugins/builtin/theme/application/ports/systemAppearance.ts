import type { Scheme } from "@/lib/appearance";
import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * The OS appearance, which the `"system"` theme id follows.
 *
 * Reached through a port because resolving a theme id to its scheme is this
 * context's rule and lives in `application/`, where a `window.matchMedia` call
 * has no business being — the rule would then need a browser to be exercised at
 * all. The adapter owns the media query.
 */
export interface SystemAppearancePort {
  scheme(): Scheme;
}

const port = createSingletonPort<SystemAppearancePort>("System appearance port is not configured");

export const configureSystemAppearancePort = port.configure;
export const systemAppearance = port.get;
