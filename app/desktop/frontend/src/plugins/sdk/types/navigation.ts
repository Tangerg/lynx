import type { ComponentType } from "react";

export type WorkIndexItemScope = "global" | "session";
export type WorkIndexItemVariant = "expanded" | "rail";

/**
 * Plugin-contributed Work Index item.
 *
 * The Work Index is the left-side agent work index, not a generic feature menu.
 * Contributors must declare whether the item is app-global or tied to the
 * current session list, and which sidebar variant it renders in — the expanded
 * source list or the collapsed rail.
 */
export interface WorkIndexItemSpec {
  id: string;
  scope: WorkIndexItemScope;
  variant: WorkIndexItemVariant;
  order?: number;
  component: ComponentType;
}
