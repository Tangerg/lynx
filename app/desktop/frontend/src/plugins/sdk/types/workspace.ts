// Workspace + layout + routes + settings panes — every "surface a
// component" plugin contribution that isn't already covered by a more
// specific file (composer, sidebar, message).

import type { ComponentType } from "react";

export interface SettingsPaneSpec {
  /** Stable id used as the rail key + storage namespace if needed. */
  id: string;
  /** Sidebar label. */
  label: string;
  /** Optional icon name (any `IconName` the host exposes). */
  icon?: string;
  /** Sort hint — lower comes first. Built-ins use 0..99; plugins ≥ 100. */
  order?: number;
  /** The pane content. Receives no props in v1. */
  component: ComponentType;
}

/** First-launch docking hint. Plain regions only; floating is not exposed. */
export type DockLocation = "left" | "right" | "main" | "bottom";

/**
 * A plugin-contributed view that participates in the workspace layout.
 * Unlike `LayoutSlotSpec`, a workspace view doesn't pick a position — the
 * user does (open, close, switch tabs). The kernel only needs `id` +
 * the component; everything else is a hint.
 */
export interface WorkspaceViewSpec {
  /** Stable id — used as the layout persistence key. */
  id: string;
  /** Tab title shown in the panel header. */
  title: string;
  /** Icon name for the tab header. */
  icon?: string;
  /** First-launch docking hint. Ignored once the user has saved a layout. */
  defaultLocation?: DockLocation;
  /** First-launch open by default. Set false for "registered but hidden" views. */
  openByDefault?: boolean;
  /** Sort hint within the default location. Lower comes first. */
  order?: number;
  /** The body component. Receives no props. */
  component: ComponentType;
}

/**
 * Plugin-contributed kernel region.
 *
 * The kernel renders `<Slot name="..."/>` for each region (sidebar, main,
 * statusbar, overlay). Plugins fill regions by registering a component +
 * sort hint. Most regions are conceptually singletons (sidebar / main)
 * but the registry allows multiple contributions so power users can
 * stack overlays without forking the kernel.
 *
 * The component receives no props — slot consumers read from app stores
 * (Zustand) and react-query hooks directly. That keeps the registration
 * descriptor flat and prevents the kernel from having to thread N props
 * down to N plugins.
 */
export interface LayoutSlotSpec {
  /** Stable id — multiple registrations to the same slot use this to dedupe. */
  id: string;
  /** Sort hint — lower comes first. Built-ins use 0..99; plugins ≥ 100. */
  order?: number;
  /** Optional className applied to the wrapper div around `component`. */
  className?: string;
  /** Component that renders the region. Receives no props. */
  component: ComponentType;
}

/**
 * A top-level route — registers a path → component pair. The router is
 * rebuilt from the registry at AppRouter mount time, so additions take
 * effect on next reload (or by calling `rebuildRouter()` from the host).
 */
export interface RouteSpec {
  /** Stable id — used as the TanStack route id. */
  id: string;
  /** URL path (TanStack syntax, e.g. "/", "/runs/$runId"). */
  path: string;
  /** Page component. */
  component: ComponentType;
  /** Sort hint — does not affect matching, only listing order. */
  order?: number;
}
