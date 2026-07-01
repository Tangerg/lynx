import { t } from "@/lib/i18n";
import { submitPendingApproval } from "@/plugins/builtin/agent/public/hitl";
import { stopActiveAgentRun } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING } from "@/plugins/sdk/kernelPoints";
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
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Enter",
      description: t("composer.key.sendDesc"),
      handler: ({ submit, event }) => {
        if (event.shiftKey) return false;
        submit();
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Enter",
      description: t("composer.key.approveDesc"),
      handler: ({ submit }) => {
        if (submitPendingApproval("approved")) return true;
        submit();
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Shift+Backspace",
      description: t("composer.key.declineDesc"),
      handler: () => submitPendingApproval("declined"),
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Escape",
      description: t("composer.key.stopDesc"),
      handler: () => stopActiveAgentRun(),
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "ArrowUp",
      description: t("composer.key.historyPrevDesc"),
      handler: ({ event }) => {
        const ta = targetTextarea(event);
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(0, ta.selectionStart).includes("\n")) return false;
        if (!recallPreviousComposerHistory()) return false;
        caretToEnd(ta);
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "ArrowDown",
      description: t("composer.key.historyNextDesc"),
      handler: ({ event }) => {
        const ta = targetTextarea(event);
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(ta.selectionEnd).includes("\n")) return false;
        if (!recallNextComposerHistory()) return false;
        caretToEnd(ta);
        return true;
      },
    });
  },
});
