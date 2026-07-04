import type { WorkspaceViewSpec } from "@/plugins/sdk";

export function diagnosticsWorkspaceView(
  component: WorkspaceViewSpec["component"],
): WorkspaceViewSpec {
  return {
    id: "diagnostics",
    title: "workspace.view.title.diagnostics",
    icon: "spark",
    order: 90,
    component,
  };
}
