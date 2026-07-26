import { workspaceNavigation } from "./ports/navigationState";
import type { ContextDockLauncherItem } from "./contextDockDestinationGroups";

// The launcher's own view id — the hub listing every dock destination.
const CONTEXT_LAUNCHER_VIEW_ID = "context";

export function openContextDockLauncher(): void {
  workspaceNavigation().openViewInDock(CONTEXT_LAUNCHER_VIEW_ID);
}

export function openContextDockDestination(item: ContextDockLauncherItem): void {
  workspaceNavigation().openViewInDock(item.viewId);
}

/**
 * Show or hide the dock.
 *
 * Reopening restores the view the user last had there; a dock that has never
 * been opened in this session lands on the launcher, which is the one surface
 * that explains what else can go there.
 */
export function toggleContextDock(): void {
  const navigation = workspaceNavigation();
  if (navigation.dockViewId()) {
    navigation.closeDockView();
    return;
  }
  navigation.openViewInDock(navigation.lastDockViewId() ?? CONTEXT_LAUNCHER_VIEW_ID);
}
