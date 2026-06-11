// App-side notification service — the non-plugin twin of host.notify
// (plugins/sdk/host.ts). Same contract: a durable entry in the
// notification feed (the Notifications workspace view) PLUS a transient
// toast. The feed exists exactly so users can scroll back through "did
// anything fail?" — an error that only toasts vanishes when dismissed.
//
// Success confirmations ("Copied", "Imported …") stay toast-only
// (toast.success directly): they're feedback on an action the user just
// watched succeed, not events worth re-reading.

import { toast } from "sonner";
import { useNotificationStore } from "@/plugins/sdk";

export interface NotifyOptions {
  /** Secondary line on the toast; folded into the feed entry's message. */
  description?: string;
  /** Feed attribution (the Notifications view's "{source} · time" line).
   *  Defaults to "app"; pass a domain name ("session", "import", …). */
  source?: string;
}

type Level = "info" | "error";

const TOAST_BY_LEVEL: Record<Level, typeof toast.info> = {
  info: toast.info,
  error: toast.error,
};

function notify(level: Level, message: string, opts?: NotifyOptions): void {
  useNotificationStore.getState().push({
    plugin: opts?.source ?? "app",
    level,
    message: opts?.description ? `${message} — ${opts.description}` : message,
  });
  TOAST_BY_LEVEL[level](message, opts?.description ? { description: opts.description } : undefined);
}

export function notifyInfo(message: string, opts?: NotifyOptions): void {
  notify("info", message, opts);
}
export function notifyError(message: string, opts?: NotifyOptions): void {
  notify("error", message, opts);
}
