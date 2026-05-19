// Built-in plugin: the default status pill sitting above the composer.
//
// Used to be hardcoded inside `components/chat/StatusPill.tsx`; now it's a
// chat.status layout slot contribution. Reads run state from `useAgentStore`
// and pulls the imperative `stop` binding off the store too — no props
// thread through ChatPanel.

import { Fragment } from "react";
import { Icon, StatusDot } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";
import { useAgentStore } from "@/state/agentStore";

function StatusPill() {
  const run = useAgentStore((s) => s.run);
  const stop = useAgentStore((s) => s.stop);

  return (
    <div className="status-wrap">
      <div className={`status-pill ${run.running ? "live" : ""}`}>
        {run.running ? (
          <Fragment>
            <span className="sp-state live">
              <StatusDot as="sp-dot" />
              <span>Step {run.step}/{run.totalSteps}</span>
            </span>
            <span className="sp-activity">{run.activity}</span>
          </Fragment>
        ) : (
          <span className="sp-state">
            <StatusDot as="sp-dot" />
            <span>Idle</span>
          </span>
        )}
        <span className="sp-spacer" />
        <span className="sp-stat" title="Context window">
          <span className="sp-ctx-bar"><div style={{ width: `${run.ctxPct}%` }} /></span>
          <span className="sp-ctx-text">
            <span className="v">{run.tokens.used}</span>{" "}
            <span className="k">/ {run.tokens.total}</span>
          </span>
        </span>
        <span className="sp-stat" title="Session cost">
          <span className="k">$</span><span className="v">{run.cost}</span>
        </span>
        {run.running && stop && (
          <button className="sp-stop" onClick={stop} title="Stop (⌘.)">
            <Icon name="stop" size={9} />Stop
          </button>
        )}
      </div>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.status-pill",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.status", {
      id: "default-pill",
      order: 0,
      component: StatusPill,
    });
  },
});
