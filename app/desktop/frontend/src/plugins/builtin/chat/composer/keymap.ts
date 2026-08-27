import { submitPendingApproval } from "@/plugins/builtin/agent/public/hitl";
import { stopCurrentRootRun } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING } from "@/plugins/sdk/kernelPoints";
import { composerKeyBindings } from "./application/composerKeyBindings";
import {
  recallNextHistoryFromKey,
  recallPreviousHistoryFromKey,
} from "./application/composerHistoryKeys";
import { recallNextComposerHistory, recallPreviousComposerHistory } from "./public/history";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

export const composerKeymap = definePlugin({
  name: "scopeapp.builtin.composer-keymap",
  setup(ctx) {
    for (const binding of composerKeyBindings({
      send: ({ submit, event }) => {
        if (event.shiftKey) return false;
        if (!runtimeCommandsAvailable()) return true;
        submit();
        return true;
      },
      approveOrSend: ({ submit }) => {
        if (!runtimeCommandsAvailable()) return true;
        if (submitPendingApproval("approved")) return true;
        submit();
        return true;
      },
      declineApproval: () =>
        runtimeCommandsAvailable() ? submitPendingApproval("declined") : true,
      stopRun: () => runtimeCommandsAvailable() && stopCurrentRootRun(),
      historyPrevious: ({ event }) =>
        recallPreviousHistoryFromKey({ event, recall: recallPreviousComposerHistory }),
      historyNext: ({ event }) =>
        recallNextHistoryFromKey({ event, recall: recallNextComposerHistory }),
    })) {
      ctx.contribute(COMPOSER_KEY_BINDING, binding);
    }
  },
});
