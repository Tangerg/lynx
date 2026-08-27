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
import { disclosureTransition } from "@/lib/motion";
import { describeErrorType } from "@/lib/rpcErrors";
import {
  openDiagnosticsView,
  openTimelineView,
} from "@/plugins/builtin/workspace/public/deeplinks";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import type { AgentProblem } from "@/plugins/builtin/agent/public/viewState";

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
// Behavior branches on the required symbolic type. An optional boolean cannot
// distinguish an explicit refusal from an omitted value.
const UNRETRYABLE: readonly string[] = ["invalid_api_key", "invalid_params", "provider_rejected"];

interface RetryCountdown {
  problem: AgentProblem | null;
  retryAfter: number;
  remaining: number;
}

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
// doesn't take the error notice down with it. The negative cue stays on the
// icon/title/action and a 1px edge rather than washing the full reading width —
// without the edge the container said nothing, so a failed run looked like an
// ordinary card that happened to have red words in it.
//
// UX review §3.3: error must not be a dead end — gives the user a
// concrete next step (Retry / Open timeline / Open diagnostics) instead
// of forcing them to scroll up and figure out the recovery themselves.
export function RunErrorBanner() {
  const t = useT();
  const error = useActiveSessionProblem();
  const send = useChatSend();
  const hasSendAction = useCanSendToAgent();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canSend = hasSendAction && runtimeAvailable;

  // Provider-requested backoff countdown (rate-limit / overload). Ticks down
  // from error.retryAfterSeconds; re-armed whenever the error changes. While
  // counting, Retry is shown but inert — don't hammer a provider that just
  // asked us to wait.
  const retryAfter = error?.retryAfterSeconds ?? 0;
  const [countdown, setCountdown] = useState<RetryCountdown>({
    problem: null,
    retryAfter: 0,
    remaining: 0,
  });
  const retryIn =
    countdown.problem === error && countdown.retryAfter === retryAfter
      ? countdown.remaining
      : retryAfter;
  useEffect(() => {
    if (retryAfter <= 0) return;
    const started = performance.now();
    const id = setInterval(() => {
      const rem = Math.max(0, Math.ceil(retryAfter - (performance.now() - started) / 1000));
      setCountdown({ problem: error, retryAfter, remaining: rem });
      if (rem <= 0) clearInterval(id);
    }, 250);
    return () => clearInterval(id);
  }, [error, retryAfter]);

  const retryText = error ? findLastUserText() : "";

  const onRetry = () => {
    if (retryIn > 0 || !canSend || !retryText) return;
    if (!send(agentTextInput(retryText))) return;
    dismissActiveSessionProblem();
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
          transition={disclosureTransition}
          className="my-2.5 grid grid-cols-[auto_1fr_auto] items-start gap-2.5 rounded-lg border border-negative-edge bg-card px-3 py-2.5 font-sans text-fg"
        >
          <Icon name="alert" size="sm" className="mt-0.5 text-negative" />
          <div className="min-w-0">
            <div className="mb-0.5 text-ui-md font-semibold text-negative">
              {t("runError.title")}
              {error.code ? ` · ${error.code}` : ""}
            </div>
            <div className="whitespace-pre-wrap break-words text-ui-md leading-body text-fg-soft">
              {error.message ?? describeErrorType(error.code) ?? t("runError.unknown")}
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
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
            iconSize="xs"
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
