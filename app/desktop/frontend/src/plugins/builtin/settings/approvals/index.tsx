// Built-in plugin: "Approvals" settings pane (B9). Registration only — the UI
// lives in ui/ (ApprovalsPane + ModeRow + RulesRow), the RPC use cases in
// application/approvalConfig.
//
// Approval is a core capability (not feature-gated per the backend), but the
// approval.* methods only exist on a B9 runtime — a pre-B9 one rejects getMode,
// so the pane degrades to an inert "unavailable" state (handled in ApprovalsPane).

import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { approvalsSettingsPane } from "./application/approvalsContributions";
import { ApprovalsPane } from "./ui/ApprovalsPane";

export default definePlugin({
  name: "lyra.builtin.approvals-pane",
  version: "1.0.0",
  setup({ host }) {
    registerSettingsPane(host, approvalsSettingsPane(ApprovalsPane));
  },
});
