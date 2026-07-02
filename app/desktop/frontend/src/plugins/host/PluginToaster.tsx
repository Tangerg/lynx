// Mounts in PluginProvider; listens for the toast event the SDK dispatches
// from `host.notify(...)` and renders animated stacked toasts via sonner —
// the de-facto Tailwind community choice for toasts (Linear / Vercel /
// Resend / shadcn default).
//
// We keep this as a separate component (not part of the SDK) so the SDK
// surface is event-only — third-party plugins don't depend on React's
// portal / motion machinery.

import type { PluginToastDetail } from "../sdk/hostToast";
import { useEffect } from "react";
import { toast, Toaster } from "sonner";
import { PLUGIN_TOAST_EVENT } from "../sdk/hostToast";

export function PluginToaster() {
  useEffect(() => {
    const onToast = (e: Event) => {
      const detail = (e as CustomEvent<PluginToastDetail>).detail;
      const fn =
        detail.level === "warn"
          ? toast.warning
          : detail.level === "error"
            ? toast.error
            : toast.info;
      fn(detail.message);
    };
    window.addEventListener(PLUGIN_TOAST_EVENT, onToast);
    return () => window.removeEventListener(PLUGIN_TOAST_EVENT, onToast);
  }, []);

  return (
    <Toaster
      position="bottom-right"
      theme="system"
      duration={4000}
      toastOptions={{
        classNames: {
          toast:
            "rounded-md border-[0.5px] border-field bg-surface text-fg shadow-[var(--shadow-popover)]",
          title: "text-[13px] font-medium",
          description: "text-[12px] text-fg-muted",
        },
      }}
    />
  );
}
