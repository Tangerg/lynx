import { ErrorBoundary, type FallbackProps } from "react-error-boundary";
import type { ReactNode } from "react";

// ChatErrorBoundary — wraps the chat surface so a render error in one
// message, code block, or mermaid diagram doesn't crash the whole tab.
//
// Backed by `react-error-boundary` (the de-facto community library) —
// we used to hand-roll the class component, but the lib already covers
// every edge case we'd want (resetKeys, onReset, onError, fallback
// component) in ~3KB. Our shell here just supplies the Chinese-language
// fallback UI and forwards `resetKey` so switching sessions clears a
// stuck card.

type Props = {
  /** Identifier (typically the active session id) that resets the
   *  boundary on change. Lets the user "escape" a stuck session by
   *  switching tabs. */
  resetKey?: unknown;
  /** Optional label included in the console log. */
  label?: string;
  children: ReactNode;
};

function ChatErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
  return (
    <div className="chat-error-card" role="alert">
      <div className="chat-error-title">渲染出错</div>
      <pre className="chat-error-message">
        {error instanceof Error ? error.message : String(error)}
      </pre>
      <div className="chat-error-actions">
        <button
          type="button"
          className="chat-error-retry"
          onClick={resetErrorBoundary}
        >
          重试
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
        // eslint-disable-next-line no-console
        console.error(
          `[chat-error-boundary] ${label ?? "chat"}:`,
          error,
          info.componentStack,
        );
      }}
    >
      {children}
    </ErrorBoundary>
  );
}
