// Built-in plugin: patches run-state (step / activity / tokens / ctxPct /
// cost) from the agent's periodic CUSTOM `lyra.telemetry` events.

import { definePlugin, patchRun } from "@/plugins/sdk";
import { CUSTOM, type TelemetryPayload } from "@/protocol/agui/customEvents";

export default definePlugin({
  name: "lyra.builtin.telemetry-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<TelemetryPayload>(CUSTOM.TELEMETRY, (value) =>
      patchRun({
        step: value.step,
        totalSteps: value.totalSteps,
        activity: value.activity,
        tokens: value.tokens,
        ctxPct: value.ctxPct,
        cost: value.cost,
      }),
    );
  },
});
