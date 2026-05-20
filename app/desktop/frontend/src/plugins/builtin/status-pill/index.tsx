// Built-in plugin: the status-bar items (run state, tokens, cost) and
// the flex spacer that pushes the right-hand items to the far edge.
//
// Previously a single fat pill above the composer; the VS Code-inspired
// layout moved these into the slim 24px status bar at the bottom.

import { Icon, StatusDot } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";
import { useAgentStore } from "@/state/agentStore";

function RunState() {
  const run = useAgentStore((s) => s.run);
  const stop = useAgentStore((s) => s.stop);
  return (
    <span className={`sb-item sb-run ${run.running ? "live" : ""}`}>
      <StatusDot as="sb-dot" />
      {run.running ? (
        <>
          <span>Step {run.step}/{run.totalSteps}</span>
          <span className="sb-sep">·</span>
          <span className="sb-activity">{run.activity}</span>
          {stop && (
            <button className="sb-stop" onClick={stop} title="Stop (⌘.)">
              <Icon name="stop" size={9} />Stop
            </button>
          )}
        </>
      ) : (
        <span>Idle</span>
      )}
    </span>
  );
}

function Spacer() {
  return <span className="sb-spacer" />;
}

function Tokens() {
  const run = useAgentStore((s) => s.run);
  return (
    <span className="sb-item" title="Context window">
      <span className="sb-ctx-bar"><div style={{ width: `${run.ctxPct}%` }} /></span>
      <span>{run.tokens.used} / {run.tokens.total}</span>
    </span>
  );
}

function Cost() {
  const run = useAgentStore((s) => s.run);
  return (
    <span className="sb-item" title="Session cost">
      <span className="sb-key">$</span>
      <span>{run.cost}</span>
    </span>
  );
}

export default definePlugin({
  name: "lyra.builtin.status-pill",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.statusbar", { id: "run",     order: 0,   component: RunState });
    host.layout.register("app.statusbar", { id: "spacer",  order: 100, component: Spacer });
    host.layout.register("app.statusbar", { id: "tokens",  order: 200, component: Tokens });
    host.layout.register("app.statusbar", { id: "cost",    order: 210, component: Cost });
  },
});
