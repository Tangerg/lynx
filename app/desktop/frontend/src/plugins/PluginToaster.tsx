// Mounts in PluginProvider; listens for the toast event the SDK dispatches
// from `host.notify(...)` and renders animated stacked toasts via sonner —
// the de-facto Tailwind community choice for toasts (Linear / Vercel /
// Resend / shadcn default).
//
// We keep this as a separate component (not part of the SDK) so the SDK
// surface is event-only — third-party plugins don't depend on React's
// portal / motion machinery.

import { useEffect } from "react";
import { Toaster, toast } from "sonner";
import { PLUGIN_TOAST_EVENT, type PluginToastDetail } from "./sdk";

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
      position="top-right"
      theme="system"
      duration={4000}
      toastOptions={{
        classNames: {
          toast: "rounded-md border border-line-soft bg-surface text-fg shadow-lg",
          title: "text-[13px] font-medium",
          description: "text-[12px] text-fg-muted",
        },
      }}
    />
  );
}
