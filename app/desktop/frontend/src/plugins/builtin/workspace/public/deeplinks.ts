// Deeplink helpers — open a known workspace view. Centralised so the view id
// stays in one place; callers (status pill RunId, RunErrorBanner actions, …) just
// call the function.
//
// Everything opened from the conversation lands in the dock, beside it: a click
// in the transcript should never cost the user the conversation. `settings` is the
// exception — it has nothing to say beside a chat.
//
// A foreign context that spells `openWorkspaceViewInDock("notifications")` has taken a
// dependency on this context's id vocabulary that nothing checks: rename the view
// and the call still compiles, the click just stops working. Three had.

import { openWorkspaceView, openWorkspaceViewInDock } from "../application/navigation";

export function openTimelineView(): void {
  openWorkspaceViewInDock("timeline");
}

export function openDiagnosticsView(): void {
  openWorkspaceViewInDock("diagnostics");
}

export function openNotificationsView(): void {
  openWorkspaceViewInDock("notifications");
}

/** The settings view itself, with no pane in mind — the work index's entry.
 *  To land on a specific pane, use the settings pack's pane ids with
 *  `openWorkspaceSettingsPane`. */
export function openSettingsView(): void {
  openWorkspaceView("settings");
}

/** The diff view in the dock, showing whatever file is already active. */
export function openDiffViewInDock(): void {
  openWorkspaceViewInDock("diff");
}
