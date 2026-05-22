// Mounts in PluginProvider; listens for the toast event the SDK dispatches
// from `host.notify(...)` and renders animated stacked toasts.
//
// We keep this as a separate component (not part of the SDK) so the SDK
// surface is event-only — third-party plugins don't depend on React's
// portal / motion machinery.

import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { nanoid } from "nanoid";
import { PLUGIN_TOAST_EVENT, type PluginToastDetail } from "./sdk";
import { swift } from "@/lib/motion";

type Toast = PluginToastDetail & { id: string };

const AUTO_DISMISS_MS = 4_000;

export function PluginToaster() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  useEffect(() => {
    const onToast = (e: Event) => {
      const detail = (e as CustomEvent<PluginToastDetail>).detail;
      const id = nanoid(8);
      setToasts((prev) => [...prev, { ...detail, id }]);
      window.setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id));
      }, AUTO_DISMISS_MS);
    };
    window.addEventListener(PLUGIN_TOAST_EVENT, onToast);
    return () => window.removeEventListener(PLUGIN_TOAST_EVENT, onToast);
  }, []);

  return (
    <div className="plugin-toaster">
      <AnimatePresence initial={false}>
        {toasts.map((t) => (
          <motion.div
            key={t.id}
            className={`plugin-toast ${t.level}`}
            initial={{ opacity: 0, y: 8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -4, scale: 0.98 }}
            transition={swift}
          >
            {t.message}
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}
