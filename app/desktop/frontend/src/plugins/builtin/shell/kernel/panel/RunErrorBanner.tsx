// Run error banner — a dismissible warning strip pinned above the message
// stream when the agent's last run ended with an error. Offers retry (resume
// the same run), timeline (open timeline view), diagnostics (open diagnostics
// view), and dismiss. Dismissing clears the error from the view state; it
// persists in the timeline regardless.
import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Icon, IconButton } from "@/ui";
import { BannerAction } from "./BannerAction";
import { flattenText } from "@/plugins/builtin/agent/public/messageContent";
import { getActiveConversationSnapshot } from "@/plugins/builtin/agent/public/conversation";
import {
  agentTextInput,
  useCanSendToAgent,
  useChatSend,
} from "@/plugins/builtin/agent/public/input";
import {
  dismissActiveSessionProblem,
  useActiveSessionProblem,
} from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { swift } from "@/lib/motion";
import { describeErrorType } from "@/lib/rpcErrors";
import {
  openDiagnosticsView,
  openTimelineView,
} from "@/plugins/builtin/workspace/public/deeplinks";

// Best-effort: find the most recent user-message plaintext so Retry can
// replay it. Returns "" if no usable text exists — Retry hides in that
// case (there's nothing to resend).
function findLastUserText(): string {
  const { messages } = getActiveConversationSnapshot();
  const last = messages.findLast((m) => m.role === "user" && flattenText(m.blocks).trim() !== "");
  return last ? flattenText(last.blocks).trim() : "";
}

// Resending the same text cannot clear these — the credential, the request
// shape, or the provider's verdict on it is what has to change.
//
// This used to read a `retryable` flag off the wire, which never worked: the
// runtime omits the field when false, so the guard could not tell "don't retry"
// from "nothing said" and was silently always true. Behavior branches on the
// symbolic type, which is the one thing a problem always carries.
const UNRETRYABLE: readonly string[] = ["invalid_api_key", "invalid_params", "provider_rejected"];

// RunErrorBanner — surfaces an run error.
//
// The reducer parks the error message on `state.error` until the next
// RUN_STARTED clears it, or until the user dismisses it explicitly.
//
// This is where the words come from when the runtime had none: `message` is
// only the per-occurrence detail the runtime actually reported, so a failure it
// classified but couldn't elaborate on (an internal error must not put its
// internals on the wire) falls through to this locale's copy for the symbol.
// The runtime deliberately does not supply that sentence — it would be one
// locale's copy authored where no translator can see it.
// Sits above the message stream so a render error inside MessageStream
// doesn't take the error notice down with it. Tinted with --color-negative
// so it reads as a stoppable problem, not a passing notice.
//
// UX review §3.3: error must not be a dead end — gives the user a
// concrete next step (Retry / Open timeline / Open diagnostics) instead
// of forcing them to scroll up and figure out the recovery themselves.
export function RunErrorBanner() {
  const t = useT();
  const error = useActiveSessionProblem();
  const send = useChatSend();
  const canSend = useCanSendToAgent();

  // Provider-requested backoff countdown (rate-limit / overload). Ticks down
  // from error.retryAfterSeconds; re-armed whenever the error changes. While
  // counting, Retry is shown but inert — don't hammer a provider that just
  // asked us to wait.
  const retryAfter = error?.retryAfterSeconds ?? 0;
  const errKey = error ? (error.code ?? error.message ?? "unknown") : null;
  const [retryIn, setRetryIn] = useState(0);
  useEffect(() => {
    if (retryAfter <= 0) {
      setRetryIn(0);
      return;
    }
    const started = performance.now();
    setRetryIn(retryAfter);
    const id = setInterval(() => {
      const rem = Math.max(0, Math.ceil(retryAfter - (performance.now() - started) / 1000));
      setRetryIn(rem);
      if (rem <= 0) clearInterval(id);
    }, 250);
    return () => clearInterval(id);
  }, [retryAfter, errKey]);

  const retryText = error ? findLastUserText() : "";

  const onRetry = () => {
    if (retryIn > 0 || !canSend || !retryText) return;
    dismissActiveSessionProblem();
    send(agentTextInput(retryText));
  };

  const canRetry = canSend && Boolean(retryText) && !UNRETRYABLE.includes(error?.code ?? "");

  return (
    <AnimatePresence initial={false}>
      {error && (
        <motion.div
          role="alert"
          initial={{ opacity: 0, y: -6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={swift}
          className="grid grid-cols-[auto_1fr_auto] items-start gap-2.5 mx-4 mt-2.5 mb-1 rounded-lg px-4 py-3 bg-negative-wash text-fg font-sans"
        >
          <Icon name="bug" size={14} className="text-negative mt-0.5" />
          <div className="min-w-0">
            <div className="text-ui-lg font-semibold text-negative mb-0.5">
              {t("runError.title")}
              {error.code ? ` · ${error.code}` : ""}
            </div>
            <div className="text-ui-lg text-fg-soft whitespace-pre-wrap break-words">
              {error.message ?? describeErrorType(error.code) ?? t("runError.unknown")}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {canRetry && (
                <BannerAction
                  icon="loop"
                  label={
                    retryIn > 0
                      ? t("runError.action.retryIn", { seconds: retryIn })
                      : t("runError.action.retry")
                  }
                  onClick={onRetry}
                  disabled={retryIn > 0}
                  primary
                />
              )}
              <BannerAction
                icon="history"
                label={t("runError.action.timeline")}
                onClick={openTimelineView}
              />
              <BannerAction
                icon="spark"
                label={t("runError.action.diagnostics")}
                onClick={openDiagnosticsView}
              />
            </div>
          </div>
          <IconButton
            icon="x"
            iconSize={12}
            size="xs"
            quiet
            title={t("runError.action.dismiss")}
            onClick={dismissActiveSessionProblem}
          />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
