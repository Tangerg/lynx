import { DataView, Icon, IconButton, ScrollArea } from "@/components/common";
import { Terminal } from "@/components/views/Terminal";
import { ViewHeader } from "@/components/views/ViewHeader";
import { useTerminal } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

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
      <span style={{ color: running ? "var(--color-info)" : "var(--color-text-faint)" }}>
        ● {running ? "Running" : "Idle"}
      </span>
      <span style={{ margin: "0 8px", color: "var(--color-text-faint)" }}>·</span>
      <span>
        {errors} error{errors === 1 ? "" : "s"} · {warnings} warning{warnings === 1 ? "" : "s"}
      </span>
      <span style={{ margin: "0 8px", color: "var(--color-text-faint)" }}>·</span>
      <span>{cwd}</span>
    </>
  );

  return (
    <>
      <ViewHeader
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
      />
      <ScrollArea>
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
      </ScrollArea>
    </>
  );
}

export const terminalView = definePlugin({
  name: "lyra.builtin.view-terminal",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "terminal",
      title: "Terminal",
      icon: "terminal",
      openByDefault: false,
      order: 10,
      component: TerminalTab,
    });
  },
});
