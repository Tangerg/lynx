// Built-in plugin: "Approvals" settings pane (B9). Registration only — the UI
// lives in ui/ (ApprovalsPane + ModeRow + RulesRow), the RPC use cases in
// application/approvalConfig.
//
// Approval is a core capability (not feature-gated per the backend), but the
// approval.* methods only exist on a B9 runtime — a pre-B9 one rejects getMode,
// so the pane degrades to an inert "unavailable" state (handled in ApprovalsPane).

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { APPROVALS_PANE } from "../public/panes";

const ApprovalsPane = lazy(() =>
  import("./ui/ApprovalsPane").then(({ ApprovalsPane }) => ({ default: ApprovalsPane })),
);

export default definePlugin({
  name: "lyra.builtin.approvals-pane",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: APPROVALS_PANE,
      label: "settings.pane.approvals",
      group: "agent",
      icon: "shield",
      order: 55,
      component: ApprovalsPane,
    });
  },
});
