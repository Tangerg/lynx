// Built-in plugin: appends an `approval` content block when the agent
// requests confirmation (CUSTOM event `lyra.approval`).

import { appendBlockToMessage, definePlugin } from "@/plugins/sdk";
import { CUSTOM, type ApprovalRequest } from "@/protocol/agui/customEvents";

export default definePlugin({
  name: "lyra.builtin.approval-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<ApprovalRequest>(CUSTOM.APPROVAL, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "approval",
        text: value.text,
        command: value.command,
        reason: value.reason,
      }),
    );
  },
});
