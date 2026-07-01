import { t } from "@/lib/i18n";
import { submitPendingApproval } from "@/lib/agent/submitPendingApproval";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING } from "@/plugins/sdk/kernelPoints";
import { useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "./adapters/composerStore";
import { useSessionStore } from "@/state/sessionStore";

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
      handler: () => {
        const sid = useSessionStore.getState().activeSessionId;
        const entry = useAgentStore.getState().sessions[sid];
        if (!entry?.view.run.running) return false;
        entry.stop?.();
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "ArrowUp",
      description: t("composer.key.historyPrevDesc"),
      handler: ({ event }) => {
        const ta = targetTextarea(event);
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(0, ta.selectionStart).includes("\n")) return false;
        if (!useComposerStore.getState().historyPrev()) return false;
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
        if (!useComposerStore.getState().historyNext()) return false;
        caretToEnd(ta);
        return true;
      },
    });
  },
});
