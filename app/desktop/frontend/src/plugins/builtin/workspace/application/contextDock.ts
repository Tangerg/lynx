import {
  workspaceNavigation,
  type WorkspaceViewTab,
} from "@/plugins/builtin/workspace/application/ports/navigationState";

export function openContextDockView(tab: WorkspaceViewTab): void {
  workspaceNavigation().openViewBeside(tab);
}

export function closeContextDockView(): void {
  workspaceNavigation().closeSplit();
}
