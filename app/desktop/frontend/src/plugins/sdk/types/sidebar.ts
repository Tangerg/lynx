// Sidebar plugin surface — expanded-sidebar sections + rail-mode items.

import type { ComponentType } from "react";

/**
 * Plugin-contributed sidebar section, rendered between the search box and
 * the user-card footer. Each section owns its own header + body; the
 * sidebar kernel just orders them by `order`.
 */
export type SidebarSectionSpec = {
  id: string;
  /** Sort hint. */
  order?: number;
  /** Section body. Receives no props. */
  component: ComponentType;
};

/**
 * Plugin-contributed item in the collapsed (rail) sidebar. The kernel
 * renders all registered items vertically in `order`. Each item may be a
 * single button, a stack of buttons, a divider, or anything else — it
 * just has to fit in the rail's narrow column.
 *
 * Conventional order ranges:
 *   - 0..99: top-area items (brand, new session, search)
 *   - 100..899: middle area (recent sessions, custom stacks)
 *   - 900..999: bottom area (tools, settings, user) — these typically
 *     render with `margin-top: auto` or similar to stick to the bottom
 */
export type SidebarRailItemSpec = {
  id: string;
  /** Sort hint — see ranges above. */
  order?: number;
  /** Item body. Receives no props. */
  component: ComponentType;
};
