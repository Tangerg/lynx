import { t } from "@/lib/i18n";
import { submitPendingApproval } from "@/plugins/builtin/agent/public/hitl";
import { stopActiveAgentRun } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING } from "@/plugins/sdk/kernelPoints";
import { composerKeyBindings } from "./application/composerContributions";
import { recallNextComposerHistory, recallPreviousComposerHistory } from "./public/history";

// After a history recall swaps the textarea value, park the caret at the end on
// the next frame so repeated arrows keep walking through shell-style history.
function caretToEnd(ta: HTMLTextAreaElement): void {
  requestAnimationFrame(() => {
    const end = ta.value.length;
    ta.setSelectionRange(end, end);
  });
}

function targetTextarea(event: KeyboardEvent): HTMLTextAreaElement | null {
  return event.target instanceof HTMLTextAreaElement ? event.target : null;
}

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
      historyPrevious: ({ event }) => {
        const ta = targetTextarea(event);
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(0, ta.selectionStart).includes("\n")) return false;
        if (!recallPreviousComposerHistory()) return false;
        caretToEnd(ta);
        return true;
      },
      historyNext: ({ event }) => {
        const ta = targetTextarea(event);
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(ta.selectionEnd).includes("\n")) return false;
        if (!recallNextComposerHistory()) return false;
        caretToEnd(ta);
        return true;
      },
    })) {
      host.extensions.contribute(COMPOSER_KEY_BINDING, binding);
    }
  },
});
