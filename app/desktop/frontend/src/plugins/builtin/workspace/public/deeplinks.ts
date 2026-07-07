// Deeplink helpers — promote a known workspace view into the chat-area
// tab strip. Centralised so the view id / title / icon stay in one
// place; callers (status pill RunId, RunErrorBanner actions, …) just
// call the function.
//
import { openWorkspaceView } from "../application/navigation";

export function openTimelineView(): void {
  openWorkspaceView("timeline");
}

export function openDiagnosticsView(): void {
  openWorkspaceView("diagnostics");
}
