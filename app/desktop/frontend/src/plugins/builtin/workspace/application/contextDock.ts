import {
  workspaceNavigation,
  type WorkspaceViewTab,
} from "@/plugins/builtin/workspace/application/ports/navigationState";
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

export function openContextDockLauncher(): void {
  openContextDockView({
    id: "context",
    title: "workspace.view.title.context",
    icon: "panel-r",
  });
}

export function openContextDockDestination(destination: ContextDockDestinationSpec): void {
  openContextDockView(contextDockDestinationTab(destination));
}

export function closeContextDockView(): void {
  workspaceNavigation().closeSplit();
}
