import { AnimatePresence, motion } from "motion/react";
import { Icon } from "@/components/common";
import { swift } from "@/lib/motion";
import { getCurrentSessionView, useAgentAction, useAgentSlice, useAgentStore } from "@/state/agentStore";
import { openDiagnosticsView, openTimelineView } from "@/state/deeplinks";
import { useSessionStore } from "@/state/sessionStore";

// Best-effort: find the most recent user-message plaintext so Retry can
// replay it. Returns "" if no usable text exists — Retry hides in that
// case (there's nothing to resend).
function findLastUserText(): string {
  const { messages } = getCurrentSessionView();
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.role !== "user") continue;
    const text = m.blocks
      .map((b) => ("text" in b ? ((b as { text?: string }).text ?? "") : ""))
      .filter(Boolean)
      .join("\n\n")
      .trim();
    if (text) return text;
  }
  return "";
}

// RunErrorBanner — surfaces an AG-UI RUN_ERROR event.
//
// The reducer parks the error message on `state.error` until the next
// RUN_STARTED clears it, or until the user dismisses it explicitly.
// Sits above the message stream so a render error inside MessageStream
// doesn't take the error notice down with it. Tinted with --color-negative
// so it reads as a stoppable problem, not a passing notice.
//
// UX review §3.3: error must not be a dead end — gives the user a
// concrete next step (Retry / Open timeline / Open diagnostics) instead
// of forcing them to scroll up and figure out the recovery themselves.
export function RunErrorBanner() {
  const error = useAgentSlice((v) => v.error);
  const sid = useSessionStore((s) => s.activeSessionId);
  const clearError = useAgentStore((s) => s.clearError);
  const send = useAgentAction("send");

  const onRetry = () => {
    if (!send) return;
    const text = findLastUserText();
    if (!text) return;
    clearError(sid);
    send(text);
  };

  const canRetry = Boolean(send) && Boolean(findLastUserText());

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
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {canRetry && <BannerAction icon="loop" label="Retry" onClick={onRetry} primary />}
              <BannerAction icon="history" label="Open timeline" onClick={openTimelineView} />
              <BannerAction icon="spark" label="Diagnostics" onClick={openDiagnosticsView} />
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

interface BannerActionProps {
  icon: "loop" | "history" | "spark";
  label: string;
  onClick: () => void;
  primary?: boolean;
}

function BannerAction({ icon, label, onClick, primary }: BannerActionProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        primary
          ? "inline-flex h-6 items-center gap-1 rounded-md border border-negative/40 bg-negative/15 px-2 font-sans text-[11.5px] font-semibold text-negative cursor-pointer transition-colors hover:bg-negative/25"
          : "inline-flex h-6 items-center gap-1 rounded-md border border-line-soft bg-transparent px-2 font-sans text-[11.5px] text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
      }
    >
      <Icon name={icon} size={11} />
      <span>{label}</span>
    </button>
  );
}
