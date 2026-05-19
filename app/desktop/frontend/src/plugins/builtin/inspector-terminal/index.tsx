import { Icon, IconButton, ScrollArea } from "@/components/common";
import { Terminal } from "@/components/inspector/Terminal";
import { useTerminal } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

function TerminalTab() {
  const { data: lines } = useTerminal();
  const title = "pnpm typecheck";
  const running = true;
  const cwd = "~/code/fern-api";
  const errors = 1;
  const warnings = 1;

  return (
    <>
      <div className="insp-head">
        <div className="ficon"><Icon name="terminal" size={14} /></div>
        <div style={{ minWidth: 0 }}>
          <div className="ftitle">{title}</div>
          <div className="fsub">
            <span style={{ color: running ? "var(--color-info)" : "var(--color-text-faint)" }}>
              ● {running ? "Running" : "Idle"}
            </span>
            <span style={{ margin: "0 8px", color: "var(--color-text-faint)" }}>·</span>
            <span>{errors} error{errors === 1 ? "" : "s"} · {warnings} warning{warnings === 1 ? "" : "s"}</span>
            <span style={{ margin: "0 8px", color: "var(--color-text-faint)" }}>·</span>
            <span>{cwd}</span>
          </div>
        </div>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton title="Re-run"><Icon name="loop" size={14} /></IconButton>
          <IconButton title="Stop"><Icon name="stop" size={12} /></IconButton>
        </div>
      </div>
      <ScrollArea><Terminal lines={lines ?? []} running={running} /></ScrollArea>
    </>
  );
}

export default definePlugin({
  name: "lyra.builtin.inspector-terminal",
  version: "1.0.0",
  setup({ host }) {
    host.inspector.registerTab({
      id: "terminal",
      label: "Terminal",
      icon: "terminal",
      order: 10,
      component: TerminalTab,
    });
  },
});
