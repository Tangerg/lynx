import { workspaceNavigation, type WorkspaceViewTab } from "./ports/navigationState";
import type { ContextDockLauncherItem } from "./contextDockDestinationGroups";

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

export function openContextDockDestination(item: ContextDockLauncherItem): void {
  openContextDockView({ id: item.viewId });
}
