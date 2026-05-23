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
// doesn't take the error notice down with it.
export function RunErrorBanner() {
  const error = useAgentSlice((v) => v.error);
  const sid = useSessionStore((s) => s.activeSessionId);
  const clearError = useAgentStore((s) => s.clearError);

  return (
    <AnimatePresence initial={false}>
      {error && (
        <motion.div
          className="run-error"
          role="alert"
          initial={{ opacity: 0, y: -6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={swift}
        >
          <Icon name="bug" size={14} />
          <div className="run-error-body">
            <div className="run-error-title">
              Agent error{error.code ? ` · ${error.code}` : ""}
            </div>
            <div className="run-error-message">{error.message}</div>
          </div>
          <button
            type="button"
            className="run-error-dismiss"
            onClick={() => clearError(sid)}
            title="Dismiss"
          >
            <Icon name="x" size={12} />
          </button>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
