// Diagnostics plugin — registers the "Diagnostics" workspace view that
// renders the local telemetry sink (traces / metrics / logs).
//
// The OTel providers are installed always-on by the bootstrap plugin
// (lib/observability, mirroring the backend's setup-at-start), NOT lazily by
// this view — traces + trace-context propagation must work whether or not
// anyone opened Diagnostics. This plugin is now a pure consumer of the
// in-memory stores.

import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { DiagnosticsView } from "./DiagnosticsView";

export default definePlugin({
  name: "lyra.builtin.diagnostics",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "diagnostics",
      title: "workspace.view.title.diagnostics",
      icon: "spark",
      order: 90,
      component: DiagnosticsView,
    });
  },
});
