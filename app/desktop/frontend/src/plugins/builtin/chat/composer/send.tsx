import { AgentIconButton } from "@/components/agent-studio";
import { Tooltip } from "@/components/common";
import { useSendComposerInput } from "./public/sendToAgent";
import { useIsAgentRunning, useStopActiveAgentRun } from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useComposerImages, useComposerPastes } from "./public/attachments";
import { useClearComposerDraft, useComposerText } from "./public/draft";
import { useRecordComposerHistory } from "./public/history";
import { submitComposer } from "./application/submitComposer";

function SendButton() {
  const t = useT();
  const value = useComposerText();
  const images = useComposerImages();
  const pastes = useComposerPastes();
  const recordHistory = useRecordComposerHistory();
  const clear = useClearComposerDraft();
  const send = useSendComposerInput();
  const stop = useStopActiveAgentRun();
  const running = useIsAgentRunning();

  if (running) {
    if (value.trim()) {
      return (
        <Tooltip label={t("composer.action.steer")}>
          <AgentIconButton
            icon="arrow-up"
            iconSize={16}
            onClick={() =>
              submitComposer({ value, clear, sendInput: send, images, pastes, recordHistory })
            }
            className="h-9 w-9 shrink-0 rounded-full bg-cta text-cta-text hover:bg-cta-hover hover:text-cta-text"
            data-slot="composer-send"
          />
        </Tooltip>
      );
    }
    return (
      <Tooltip label={t("composer.action.stop")}>
        <AgentIconButton
          icon="stop"
          iconSize={13}
          disabled={!stop}
          onClick={() => stop?.()}
          className="h-9 w-9 shrink-0 rounded-full bg-surface-2 text-fg-muted hover:bg-surface-3"
          data-slot="composer-stop"
        />
      </Tooltip>
    );
  }

  const disabled = !value.trim() && images.length === 0 && pastes.length === 0;
  const onClick = () =>
    submitComposer({ value, clear, sendInput: send, images, pastes, recordHistory });

  return (
    <Tooltip label={t("composer.action.send")}>
      <AgentIconButton
        icon="arrow-up"
        iconSize={16}
        disabled={disabled}
        onClick={onClick}
        className={cn(
          "h-9 w-9 shrink-0 rounded-full",
          disabled
            ? "bg-surface-2 text-fg-faint cursor-not-allowed"
            : "bg-cta text-cta-text hover:bg-cta-hover hover:text-cta-text",
        )}
        data-slot="composer-send"
      />
    </Tooltip>
  );
}

export const composerSend = definePlugin({
  name: "lyra.builtin.composer-send",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.end", { id: "send", order: 100, component: SendButton });
  },
});
