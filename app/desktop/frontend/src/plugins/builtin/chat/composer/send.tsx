import { IconButton } from "@/ui";
import { useSendComposerInput } from "./public/sendToAgent";
import { useIsCurrentRootRunning, useStopCurrentRootRun } from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useComposerImages, useComposerPastes } from "./public/attachments";
import { useClearComposerDraft, useComposerText } from "./public/draft";
import { useRecordComposerHistory } from "./public/history";
import { composerActionLayout } from "./application/composerActionLayout";
import { composerSendSlot } from "./application/composerContributions";
import { submitComposer } from "./application/submitComposer";

// The composer's action target: one control whose glyph changes across steer /
// send / stop, so the place you click never moves. A rounded square on the
// control ladder rather than a circle — every other control in this language is
// a rounded rectangle, and a lone disc beside them reads as a different kit.
const ACTION =
  "size-[var(--control-height-md)] shrink-0 rounded-[var(--button-radius)] bg-cta text-cta-text hover:bg-cta-hover hover:text-cta-text active:translate-y-[0.5px]";
const ACTION_OFF =
  "size-[var(--control-height-md)] shrink-0 rounded-[var(--button-radius)] bg-surface-2 text-fg-faint";
const QUIET = "size-[var(--control-height-md)] shrink-0 rounded-[var(--button-radius)]";

function SendButton() {
  const t = useT();
  const value = useComposerText();
  const images = useComposerImages();
  const pastes = useComposerPastes();
  const recordHistory = useRecordComposerHistory();
  const clear = useClearComposerDraft();
  const send = useSendComposerInput();
  const stop = useStopCurrentRootRun();
  const running = useIsCurrentRootRunning();

  const hasInput = Boolean(value.trim()) || images.length > 0 || pastes.length > 0;
  const layout = composerActionLayout({ running, hasInput });
  const submit = () =>
    submitComposer({ value, clear, sendInput: send, images, pastes, recordHistory });

  const stopButton = (primary: boolean) => (
    <IconButton
      icon="stop"
      iconSize="xs"
      press={false}
      disabled={!stop}
      title={t("composer.action.stop")}
      onClick={() => stop?.()}
      className={primary ? (stop ? ACTION : ACTION_OFF) : QUIET}
    />
  );

  const submitButton = (label: string, enabled: boolean) => (
    <IconButton
      icon="arrow-up"
      iconSize="sm"
      press={false}
      disabled={!enabled}
      title={label}
      onClick={submit}
      className={enabled ? ACTION : ACTION_OFF}
    />
  );

  return (
    <>
      {layout.secondary === "stop" && stopButton(false)}
      {layout.primary === "stop" && stopButton(true)}
      {layout.primary === "steer" && submitButton(t("composer.action.steer"), true)}
      {layout.primary === "send" && submitButton(t("composer.action.send"), hasInput)}
    </>
  );
}

export const composerSend = definePlugin({
  name: "lyra.builtin.composer-send",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.end", composerSendSlot(SendButton));
  },
});
