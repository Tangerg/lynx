import { t } from "@/lib/i18n";
import { submitPendingApproval } from "@/plugins/builtin/agent/public/hitl";
import { stopActiveAgentRun } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING } from "@/plugins/sdk/kernelPoints";
import { composerKeyBindings } from "./application/composerContributions";
import {
  recallNextHistoryFromKey,
  recallPreviousHistoryFromKey,
} from "./application/composerHistoryKeys";
import { recallNextComposerHistory, recallPreviousComposerHistory } from "./public/history";

export const composerKeymap = definePlugin({
  name: "lyra.builtin.composer-keymap",
  version: "1.0.0",
  setup({ host }) {
    for (const binding of composerKeyBindings(t, {
      send: ({ submit, event }) => {
        if (event.shiftKey) return false;
        submit();
        return true;
      },
      approveOrSend: ({ submit }) => {
        if (submitPendingApproval("approved")) return true;
        submit();
        return true;
      },
      declineApproval: () => submitPendingApproval("declined"),
      stopRun: () => stopActiveAgentRun(),
      historyPrevious: ({ event }) =>
        recallPreviousHistoryFromKey({ event, recall: recallPreviousComposerHistory }),
      historyNext: ({ event }) =>
        recallNextHistoryFromKey({ event, recall: recallNextComposerHistory }),
    })) {
      host.extensions.contribute(COMPOSER_KEY_BINDING, binding);
    }
  },
});
