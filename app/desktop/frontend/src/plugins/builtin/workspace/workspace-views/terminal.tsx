import { EmptyState } from "@/components/common";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

// The runtime doesn't stream process output to the client yet (see
// docs/ui-research/BACKEND-DEPENDENCIES.md B1). Rather than fixture data, the
// pane shows an honest "not wired up" state; it lights up once the backend
// surfaces a live command-output stream.
function TerminalTab() {
  return (
    <WorkspaceViewLayout icon="terminal" title="Terminal">
      <EmptyState
        icon="terminal"
        title="Terminal isn't wired up yet"
        sub="The runtime doesn't stream process output to the client yet — this pane lights up once it does."
      />
    </WorkspaceViewLayout>
  );
}

export const terminalView = defineWorkspaceView({
  id: "terminal",
  title: "Terminal",
  icon: "terminal",
  openByDefault: false,
  order: 10,
  splittable: true,
  component: TerminalTab,
});
