import { Icon, Tooltip } from "@/components/common";
import { useChatSend } from "@/lib/agent/useChatSend";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentAction, useAgentRunning } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { submitComposer } from "./application/submitComposer";

function SendButton() {
  const t = useT();
  const value = useComposerStore((s) => s.value);
  const images = useComposerStore((s) => s.images);
  const clear = useComposerStore((s) => s.clear);
  const send = useChatSend();
  const stop = useAgentAction("stop");
  const running = useAgentRunning();

  if (running) {
    if (value.trim()) {
      return (
        <Tooltip label={t("composer.action.steer")}>
          <button
            type="button"
            onClick={() => submitComposer({ value, clear, sendInput: send, images })}
            className="grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 bg-accent text-on-accent transition-transform duration-150 active:scale-95"
            data-slot="composer-send"
          >
            <Icon name="send-arrow" size={14} strokeWidth={2.5} />
          </button>
        </Tooltip>
      );
    }
    return (
      <Tooltip label={t("composer.action.stop")}>
        <button
          type="button"
          disabled={!stop}
          onClick={() => stop?.()}
          className="grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 bg-surface-3 text-fg-muted transition-colors duration-150 hover:bg-surface-4 hover:text-fg active:scale-95 disabled:cursor-not-allowed disabled:opacity-40"
          data-slot="composer-stop"
        >
          <Icon name="stop" size={13} />
        </button>
      </Tooltip>
    );
  }

  const disabled = !value.trim() && images.length === 0;
  const onClick = () => submitComposer({ value, clear, sendInput: send, images });

  return (
    <Tooltip label={t("composer.action.send")}>
      <button
        type="button"
        disabled={disabled}
        onClick={onClick}
        className={cn(
          "grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 transition-transform duration-150",
          disabled
            ? "bg-transparent text-fg-faint/30 cursor-not-allowed"
            : "bg-fg text-on-fg active:scale-95",
        )}
        data-slot="composer-send"
      >
        <Icon name="send-arrow" size={14} strokeWidth={2.5} />
      </button>
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
