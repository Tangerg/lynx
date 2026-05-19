// Built-in plugin: appends a `code` content block when the agent proposes
// a code snippet (CUSTOM event `lyra.code-proposal`).

import { appendBlockToMessage, definePlugin } from "@/plugins/sdk";
import { CUSTOM, type CodeProposalPayload } from "@/protocol/agui/customEvents";

export default definePlugin({
  name: "lyra.builtin.code-proposal-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<CodeProposalPayload>(CUSTOM.CODE_PROPOSAL, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "code",
        lang: value.lang,
        file: value.file,
        text: value.text,
      }),
    );
  },
});
