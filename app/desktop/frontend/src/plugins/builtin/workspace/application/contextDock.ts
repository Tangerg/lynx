import { workspaceNavigation, type WorkspaceViewTab } from "./ports/navigationState";
import type { ContextDockDestinationSpec } from "@/plugins/sdk";

export function contextDockDestinationTab(
  destination: ContextDockDestinationSpec,
): WorkspaceViewTab {
  return {
    id: destination.id,
    title: destination.title,
    icon: destination.icon ?? "panel-r",
  };
}

export function openContextDockView(tab: WorkspaceViewTab): void {
  workspaceNavigation().openViewBeside(tab);
}

// The launcher's reserved view id — the generic "open the dock" entry.
const CONTEXT_LAUNCHER_TAB: WorkspaceViewTab = {
  id: "context",
  title: "workspace.view.title.context",
  icon: "panel-r",
};

export function openContextDockLauncher(): void {
  openContextDockView(CONTEXT_LAUNCHER_TAB);
}

export function openContextDockDestination(destination: ContextDockDestinationSpec): void {
  openContextDockView(contextDockDestinationTab(destination));
}

export function closeContextDockView(): void {
  workspaceNavigation().closeSplit();
}
