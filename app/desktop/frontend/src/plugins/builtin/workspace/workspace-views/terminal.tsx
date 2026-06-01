import { DataView, Icon, IconButton } from "@/components/common";
import { Terminal } from "./views/Terminal";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useTerminal } from "@/lib/data/queries";
import { defineWorkspaceView } from "./defineWorkspaceView";

function TerminalTab() {
  const { data: lines, isLoading } = useTerminal();

  // TODO: surface real process metadata once the terminal data layer
  // exposes it. Mocked for the design preview today.
  const title = "pnpm typecheck";
  const running = true;
  const cwd = "~/code/fern-api";
  const errors = 1;
  const warnings = 1;

  const sub = (
    <>
      <span className={running ? "text-info" : "text-fg-faint"}>
        ● {running ? "Running" : "Idle"}
      </span>
      <span className="mx-2">·</span>
      <span>
        {errors} error{errors === 1 ? "" : "s"} · {warnings} warning{warnings === 1 ? "" : "s"}
      </span>
      <span className="mx-2">·</span>
      <span>{cwd}</span>
    </>
  );

  return (
    <WorkspaceViewLayout
      icon="terminal"
      title={title}
      sub={sub}
      actions={
        <>
          <IconButton title="Re-run">
            <Icon name="loop" size={14} />
          </IconButton>
          <IconButton title="Stop">
            <Icon name="stop" size={12} />
          </IconButton>
        </>
      }
    >
      <DataView
        items={lines}
        isLoading={isLoading}
        skeletonCount={8}
        empty={{
          icon: "terminal",
          title: "No output yet",
          sub: "Run a command — terminal output will stream here.",
        }}
      >
        {(rows) => <Terminal lines={rows} running={running} />}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const terminalView = defineWorkspaceView({
  id: "terminal",
  title: "Terminal",
  icon: "terminal",
  openByDefault: false,
  order: 10,
  component: TerminalTab,
});
