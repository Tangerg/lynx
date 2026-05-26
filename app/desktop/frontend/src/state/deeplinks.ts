// Deeplink helpers — promote a known workspace view into the chat-area
// tab strip. Centralised so the view id / title / icon stay in one
// place; callers (status pill RunId, RunErrorBanner actions, …) just
// call the function.
//
// Lives in state/ (not plugins/sdk) because the call surface is "do a
// store mutation"; importers shouldn't need to know about the plugin
// registry to navigate the UI.

import { useSessionStore } from "./sessionStore";

export function openTimelineView(): void {
  useSessionStore.getState().openMainView({
    id: "timeline",
    title: "Timeline",
    icon: "history",
  });
}

export function openDiagnosticsView(): void {
  useSessionStore.getState().openMainView({
    id: "diagnostics",
    title: "Diagnostics",
    icon: "spark",
  });
}
