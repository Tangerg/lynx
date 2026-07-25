// Deeplink helpers — promote a known workspace view into the chat-area
// tab strip. Centralised so the view id / title / icon stay in one
// place; callers (status pill RunId, RunErrorBanner actions, …) just
// call the function.
//
// A foreign context that spells `openWorkspaceView("notifications")` has taken a
// dependency on this context's id vocabulary that nothing checks: rename the view
// and the call still compiles, the click just stops working. Three had.

import { openWorkspaceView, openWorkspaceViewBeside } from "../application/navigation";

export function openTimelineView(): void {
  openWorkspaceView("timeline");
}

export function openDiagnosticsView(): void {
  openWorkspaceView("diagnostics");
}

export function openNotificationsView(): void {
  openWorkspaceView("notifications");
}

/** The settings view itself, with no pane in mind — the work index's entry.
 *  To land on a specific pane, use the settings pack's pane ids with
 *  `openWorkspaceSettingsPane`. */
export function openSettingsView(): void {
  openWorkspaceView("settings");
}

/** The diff view in the split, showing whatever file is already active. */
export function openDiffViewBeside(): void {
  openWorkspaceViewBeside("diff");
}
