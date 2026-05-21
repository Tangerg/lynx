import { Component, type ErrorInfo, type ReactNode } from "react";

// ChatErrorBoundary — wraps the chat surface so a render error in one
// message, code block, or mermaid diagram doesn't crash the whole tab.
//
// Why a separate boundary (vs. the existing PluginBoundary): plugin
// boundaries scope plugin renderers; the chat panel includes a lot of
// non-plugin code (smooth-text engine, react-markdown, Shiki/Mermaid
// loaders) that can throw on bad input. A dedicated boundary at the
// session level catches all of those without conflating the two
// failure domains.
//
// The boundary takes a `resetKey` — usually the active session id —
// and clears its error state when the key changes. So if session A
// blows up and the user switches to session B, B doesn't inherit A's
// error card.

type Props = {
  /** Identifier (typically the active session id) that resets the
   *  boundary on change. Lets the user "escape" a stuck session by
   *  switching tabs. */
  resetKey?: unknown;
  /** Optional label included in the error message + the console log. */
  label?: string;
  children: ReactNode;
};

type State = { error: Error | null };

export class ChatErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // eslint-disable-next-line no-console
    console.error(
      `[chat-error-boundary] ${this.props.label ?? "chat"}:`,
      error,
      info.componentStack,
    );
  }

  componentDidUpdate(prev: Props) {
    // Reset on session swap — see top-of-file note.
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null });
    }
  }

  private handleRetry = () => {
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      return (
        <div className="chat-error-card" role="alert">
          <div className="chat-error-title">渲染出错</div>
          <pre className="chat-error-message">
            {this.state.error.message || String(this.state.error)}
          </pre>
          <div className="chat-error-actions">
            <button
              type="button"
              className="chat-error-retry"
              onClick={this.handleRetry}
            >
              重试
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
