import { IconButton } from "@/ui";
import { useSendComposerInput } from "./public/sendToAgent";
import { useIsCurrentRootRunning, useStopCurrentRootRun } from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { useComposerImages, useComposerPastes } from "./public/attachments";
import { useClearComposerDraft, useComposerText } from "./public/draft";
import { useRecordComposerHistory } from "./public/history";
import { composerActionLayout } from "./application/composerActionLayout";
import { submitComposer } from "./application/submitComposer";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { useCanSendToAgent } from "@/plugins/builtin/agent/public/input";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

// The composer's action target: one control whose glyph changes across steer /
// send / stop, so the place you click never moves.
//
// A DISC, and deliberately the only one in the window. The
// controls beside it are ghosts: no fill, no edge, they are labels you can press.
// This one is the solid primary, and being a different shape is how it says so
// without shouting in colour. The reference sits its send on the same row of
// ghost dropdowns and makes it round for exactly this reason.
const ACTION =
  "size-[var(--control-height-md)] shrink-0 rounded-full bg-cta text-cta-text hover:bg-cta-hover hover:text-cta-text active:translate-y-[0.5px]";
const ACTION_OFF =
  "size-[var(--control-height-md)] shrink-0 rounded-full bg-surface-2 text-fg-faint";
const QUIET = "size-[var(--control-height-md)] shrink-0 rounded-full";

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
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const agentCanSend = useCanSendToAgent();
  const activeSessionId = useActiveSessionId();
  const canSend = runtimeAvailable && agentCanSend;

  const hasInput = Boolean(value.trim()) || images.length > 0 || pastes.length > 0;
  const layout = composerActionLayout({ running, hasInput });
  const submit = () =>
    submitComposer({
      value,
      clear,
      sendInput: send,
      images,
      pastes,
      recordHistory,
      canSend: runtimeCommandsAvailable,
    });

  const stopButton = (primary: boolean) => (
    <IconButton
      icon="stop"
      iconSize="xs"
      press={false}
      disabled={!stop || !runtimeAvailable}
      title={t("composer.action.stop")}
      onClick={() => stop?.()}
      className={primary ? (stop && runtimeAvailable ? ACTION : ACTION_OFF) : QUIET}
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
      {layout.primary === "steer" && submitButton(t("composer.action.steer"), canSend)}
      {layout.primary === "send" &&
        submitButton(
          activeSessionId ? t("composer.action.send") : t("composer.project.required"),
          canSend && hasInput,
        )}
    </>
  );
}

export const composerSend = definePlugin({
  name: "scopeapp.builtin.composer-send",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.end", {
      id: "send",
      order: 100,
      component: SendButton,
    });
  },
});
