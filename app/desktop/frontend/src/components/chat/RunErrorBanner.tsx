import { motion, AnimatePresence } from "motion/react";
import { Icon } from "@/components/common";
import { swift } from "@/lib/motion";
import { useAgentSlice, useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

// RunErrorBanner — surfaces an AG-UI RUN_ERROR event.
//
// The reducer parks the error message on `state.error` until the next
// RUN_STARTED clears it, or until the user dismisses it explicitly.
// Sits above the message stream so a render error inside MessageStream
// doesn't take the error notice down with it. Tinted with --color-negative
// so it reads as a stoppable problem, not a passing notice.
export function RunErrorBanner() {
  const error = useAgentSlice((v) => v.error);
  const sid = useSessionStore((s) => s.activeSessionId);
  const clearError = useAgentStore((s) => s.clearError);

  return (
    <AnimatePresence initial={false}>
      {error && (
        <motion.div
          role="alert"
          initial={{ opacity: 0, y: -6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={swift}
          className="grid grid-cols-[auto_1fr_auto] items-start gap-2.5 mx-4 mt-2.5 mb-1 rounded-lg px-3 py-2.5 bg-negative/12 border border-negative/35 text-fg font-sans"
        >
          <Icon name="bug" size={14} className="text-negative mt-0.5" />
          <div className="min-w-0">
            <div className="text-[13px] font-semibold text-negative mb-0.5">
              Agent error{error.code ? ` · ${error.code}` : ""}
            </div>
            <div className="text-[14px] text-fg-soft whitespace-pre-wrap break-words">
              {error.message}
            </div>
          </div>
          <button
            type="button"
            onClick={() => clearError(sid)}
            title="Dismiss"
            className="grid h-5.5 w-5.5 place-items-center rounded text-fg-faint cursor-pointer bg-transparent border-0 transition-all duration-150 hover:bg-[color-mix(in_srgb,var(--color-text)_10%,transparent)] hover:text-fg active:scale-90"
          >
            <Icon name="x" size={12} />
          </button>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
