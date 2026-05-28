import type { ErrorInfo, ReactNode } from "react";
import { Component } from "react";
import { pickPluginErrorFallback, reportPluginError } from "./sdk";

interface Props {
  /** Plugin name — used for the fallback label and the console log. */
  plugin: string;
  /** Optional label shown to the user. Defaults to the plugin name. */
  label?: string;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * Error boundary wrapped around every plugin-contributed component.
 *
 * Failure mode:
 *   - the misbehaving region renders a small red note (default) OR a
 *     plugin-provided fallback registered via
 *     `host.plugins.registerErrorFallback(...)`
 *   - main app keeps running
 *   - console gets the full stack, the error store gets a structured entry
 *
 * This is the cheapest insurance we can buy for trust-on-install plugins.
 */
export class PluginBoundary extends Component<Props, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(`[plugin] ${this.props.plugin} render failed:`, error, info.componentStack);
    reportPluginError(this.props.plugin, "render", error, info.componentStack ?? undefined);
  }

  override render(): ReactNode {
    if (!this.state.error) return this.props.children;

    const fallback = pickPluginErrorFallback();
    if (fallback) {
      const Body = fallback.component;
      // Render OUTSIDE another PluginBoundary — if the fallback itself
      // throws, React will surface that via the next enclosing boundary
      // (or crash, which is fine: the host should ship a working fallback).
      return <Body plugin={this.props.plugin} label={this.props.label} error={this.state.error} />;
    }

    return (
      <div className="plugin-boundary-error">
        <strong>{this.props.label ?? this.props.plugin}</strong> failed to render.
        <code>{this.state.error.message}</code>
      </div>
    );
  }
}
