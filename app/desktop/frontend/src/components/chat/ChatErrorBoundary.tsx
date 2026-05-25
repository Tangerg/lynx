import type { ReactNode } from "react";
import type {FallbackProps} from "react-error-boundary";
import { ErrorBoundary  } from "react-error-boundary";
import { useT } from "@/lib/i18n";

// ChatErrorBoundary — wraps the chat surface so a render error in one
// message, code block, or mermaid diagram doesn't crash the whole tab.
//
// Backed by `react-error-boundary` (the de-facto community library) —
// we used to hand-roll the class component, but the lib already covers
// every edge case we'd want (resetKeys, onReset, onError, fallback
// component) in ~3KB. Our shell here just supplies the localized
// fallback UI and forwards `resetKey` so switching sessions clears a
// stuck card.

interface Props {
  /** Identifier (typically the active session id) that resets the
   *  boundary on change. Lets the user "escape" a stuck session by
   *  switching tabs. */
  resetKey?: unknown;
  /** Optional label included in the console log. */
  label?: string;
  children: ReactNode;
}

function ChatErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
  const t = useT();
  return (
    <div
      role="alert"
      className="m-8 max-w-[720px] rounded-lg border border-negative/45 bg-negative/10 px-5 py-4 text-fg"
    >
      <div className="mb-2 font-semibold text-[15px] tracking-tight text-negative">
        {t("chat.error.title")}
      </div>
      <pre className="m-0 mb-3 max-h-[200px] overflow-auto rounded-md bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] px-3 py-2.5 font-mono text-[13px] leading-[1.55] text-fg-muted whitespace-pre-wrap break-words">
        {error instanceof Error ? error.message : String(error)}
      </pre>
      <div className="flex gap-2">
        <button
          type="button"
          onClick={resetErrorBoundary}
          className="rounded-md border border-[color-mix(in_srgb,var(--color-text)_12%,transparent)] bg-surface-2 px-3 py-1 text-[13px] text-fg font-sans cursor-pointer transition-colors hover:bg-surface-3"
        >
          {t("chat.error.retry")}
        </button>
      </div>
    </div>
  );
}

export function ChatErrorBoundary({ resetKey, label, children }: Props) {
  return (
    <ErrorBoundary
      FallbackComponent={ChatErrorFallback}
      resetKeys={resetKey === undefined ? [] : [resetKey]}
      onError={(error, info) => {
         
        console.error(`[chat-error-boundary] ${label ?? "chat"}:`, error, info.componentStack);
      }}
    >
      {children}
    </ErrorBoundary>
  );
}
