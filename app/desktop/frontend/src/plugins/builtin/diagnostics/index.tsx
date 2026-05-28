// Diagnostics plugin — registers the "Diagnostics" workspace view and
// the lazy MeterProvider installer that view uses on first open.
//
// Why opt-in by view rather than always-on instrumentation:
//   - No MeterProvider registered (plugin loaded but view never
//     opened) → every `measure*` call in lib/metrics is a JS-level
//     no-op (otel-api returns proxy meters that swallow records).
//   - View first mounted → ensureProvider() runs once; subsequent
//     mounts hit the cached promise. Provider stays installed for
//     the rest of the session so history persists across tab
//     close/reopen.
//   - Plugin unloaded → teardownProvider() shuts the SDK down and
//     otel-api falls back to no-op proxies again.

import { definePlugin } from "@/plugins/sdk";
import { DiagnosticsView } from "./DiagnosticsView";
import { teardownProvider } from "./provider";

export default definePlugin({
  name: "lyra.builtin.diagnostics",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "diagnostics",
      title: "Diagnostics",
      icon: "spark",
      openByDefault: false,
      order: 90,
      component: DiagnosticsView,
    });
    return async () => {
      await teardownProvider();
    };
  },
});
