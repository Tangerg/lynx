// Built-in plugin: queued-message strip above the composer.
//
// Messages typed while a run streams are queued (useChatSend → queueStore) and
// auto-sent when the run settles (T2.1). This surfaces the queue as removable
// chips in the `chat.status` slot so the user can see what's pending and drop
// any of it before it sends.

import type { QueuedMessage } from "@/state/queueStore";
import { Icon, Tooltip } from "@/components/common";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useQueueStore } from "@/state/queueStore";
import { useSessionStore } from "@/state/sessionStore";

// Stable empty reference — an inline `?? []` would mint a new array each render
// and break Zustand's reference-equality bail-out (infinite re-render).
const EMPTY: QueuedMessage[] = [];

function QueuedMessages() {
  const t = useT();
  const sid = useSessionStore((s) => s.activeSessionId);
  const queued = useQueueStore((s) => s.queued[sid] ?? EMPTY);
  const remove = useQueueStore((s) => s.remove);
  if (queued.length === 0) return null;
  return (
    <div className="mb-1.5 flex flex-col gap-1">
      <div className="px-1 text-[10.5px] font-medium uppercase tracking-wide text-fg-faint">
        {t("composer.queued.hint", { count: queued.length })}
      </div>
      {queued.map((m) => (
        <div
          key={m.id}
          className="group flex items-center gap-2 rounded-md border border-line-soft bg-surface-2/60 px-2.5 py-1 text-[12.5px] text-fg-muted"
        >
          <span className="min-w-0 flex-1 truncate">{m.text || t("composer.queued.image")}</span>
          <Tooltip label={t("composer.queued.remove")}>
            <button
              type="button"
              aria-label={t("composer.queued.remove")}
              onClick={() => remove(sid, m.id)}
              className="grid h-4 w-4 shrink-0 place-items-center rounded-full border-0 bg-transparent text-fg-faint opacity-0 transition-opacity hover:text-fg group-hover:opacity-100"
            >
              <Icon name="x" size={9} />
            </button>
          </Tooltip>
        </div>
      ))}
    </div>
  );
}

export const composerQueue = definePlugin({
  name: "lyra.builtin.composer-queue",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.status", { id: "queue", order: 10, component: QueuedMessages });
  },
});
