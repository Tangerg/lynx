import { AgentIconButton } from "@/ui/agent";
import { Tooltip } from "@/ui";
import { useSendComposerInput } from "./public/sendToAgent";
import { useIsAgentRunning, useStopActiveAgentRun } from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useComposerImages, useComposerPastes } from "./public/attachments";
import { useClearComposerDraft, useComposerText } from "./public/draft";
import { useRecordComposerHistory } from "./public/history";
import { composerSendSlot } from "./application/composerContributions";
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

  // Filled dark circle for the primary action; a quiet surface disc when the
  // action is unavailable. The circle is the same in steer / send / stop — only
  // the glyph changes — so the composer's action target reads as one control.
  const circle =
    "h-9 w-9 shrink-0 rounded-full bg-cta text-cta-text hover:bg-cta-hover hover:text-cta-text active:translate-y-[0.5px]";
  const circleOff = "h-9 w-9 shrink-0 rounded-full bg-surface-3 text-fg-faint";

  if (running) {
    if (value.trim()) {
      return (
        <Tooltip label={t("composer.action.steer")}>
          <AgentIconButton
            icon="arrow-up"
            iconSize={18}
            press={false}
            onClick={() =>
              submitComposer({ value, clear, sendInput: send, images, pastes, recordHistory })
            }
            className={circle}
            data-slot="composer-send"
          />
        </Tooltip>
      );
    }
    return (
      <Tooltip label={t("composer.action.stop")}>
        <AgentIconButton
          icon="stop"
          iconSize={12}
          press={false}
          disabled={!stop}
          onClick={() => stop?.()}
          className={stop ? circle : circleOff}
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
        iconSize={18}
        press={false}
        disabled={disabled}
        onClick={onClick}
        className={disabled ? circleOff : circle}
        data-slot="composer-send"
      />
    </Tooltip>
  );
}

export const composerSend = definePlugin({
  name: "lyra.builtin.composer-send",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.end", composerSendSlot(SendButton));
  },
});
