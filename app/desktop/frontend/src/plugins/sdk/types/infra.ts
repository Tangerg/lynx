// Kernel infrastructure surface — anything that's a transport,
// observability, or background service rather than a user-facing
// component contribution.

import type { AbstractAgent } from "@ag-ui/client";
import type { ComponentType } from "react";

// ---------------------------------------------------------------------------
// Notifications — persistent feed surfaced by host.notify().
// ---------------------------------------------------------------------------

export type NotificationLevel = "info" | "warn" | "error";

/**
 * One entry in the persistent notification feed. Created every time a
 * plugin calls `host.notify(...)`. The transient toast is just the visual
 * surface; the feed is what plugins (e.g. workspace view, settings pane)
 * read.
 */
export interface NotificationEntry {
  /** Monotonic id assigned by the host. */
  id: number;
  /** Plugin that called `host.notify`. */
  plugin: string;
  level: NotificationLevel;
  message: string;
  /** Created-at timestamp (ms). */
  timestamp: number;
  /** Set when the user dismisses the toast / clears the feed entry. */
  dismissed?: boolean;
}

// ---------------------------------------------------------------------------
// Logger — structured logger passed to a plugin in setup().
// ---------------------------------------------------------------------------

export type LogLevel = "debug" | "info" | "warn" | "error";

/**
 * One log event. `plugin` records who emitted it, so a UI that consumes
 * logs (notifications pane, dev panel) can group / filter by plugin.
 */
export interface LogEvent {
  plugin: string;
  level: LogLevel;
  args: unknown[];
  timestamp: number;
}

/** Subscriber for log events. Errors thrown inside are caught by the host. */
export type LogSubscriber = (event: LogEvent) => void;

// ---------------------------------------------------------------------------
// RPC hooks — runtime extension of the host.rpc namespace.
// ---------------------------------------------------------------------------

/**
 * A `beforeRequest` hook — runs immediately before the underlying fetch.
 * Can mutate the Request (set headers, replace URL, log) or return a
 * brand-new one to substitute. Awaited.
 *
 * Hooks run in registration order; the first registered runs first.
 */
export type RpcBeforeRequestHook = (request: Request) => void | Request | Promise<void | Request>;

/**
 * An `afterResponse` hook — runs once the underlying fetch resolves
 * (success or HTTP error). Can inspect / replace the Response (e.g.
 * shape error bodies, refresh expired tokens then retry).
 */
export type RpcAfterResponseHook = (
  request: Request,
  response: Response,
) => void | Response | Promise<void | Response>;

// ---------------------------------------------------------------------------
// Data providers — pluggable fetchers behind React Query hooks.
// ---------------------------------------------------------------------------

/**
 * A data fetcher registered against a key. TanStack-Query hooks in the app
 * resolve their `queryFn` by looking up the provider for their key. The
 * fetcher must return a fully-typed result, but the registry erases that
 * type so all providers fit in one map — call sites cast on their way out.
 *
 * Plugins can swap the underlying transport (HTTP, IPC, in-memory mock)
 * without callers having to know.
 */
export interface DataProviderSpec<T = unknown> {
  /** Query key — must match the consumer hook's expected key. */
  key: string;
  /** Async fetcher. Throw for failure; TanStack-Query handles the rest. */
  fetcher: () => Promise<T>;
}

// ---------------------------------------------------------------------------
// Agent sources — transports that drive the chat (HTTP, mock, IPC…).
// ---------------------------------------------------------------------------

/**
 * A provider for the AG-UI agent that drives the chat. The default ships an
 * HttpAgent against the local Go backend; alternative sources can implement
 * a WebSocket variant, mock streamer, etc.
 *
 * Only one source is active at a time — kernel-chat resolves to the
 * first spec sorted by `priority`. Higher priority wins; a user plugin
 * can override the built-in by registering at priority > 0.
 */
export interface AgentSourceSpec {
  id: string;
  label: string;
  /** Higher wins. Built-in defaults use 0. */
  priority?: number;
  /** Build a fresh agent for each session. */
  factory: () => AbstractAgent;
}

// ---------------------------------------------------------------------------
// Plugin error fallback — UI shown when a plugin component throws.
// ---------------------------------------------------------------------------

/**
 * Props passed to the registered error-fallback renderer when a plugin
 * component throws inside `PluginBoundary`.
 */
export interface PluginErrorFallbackProps {
  /** Plugin name / context label, e.g. "view:diff" or "layout:app.main:chat". */
  plugin: string;
  /** Optional human-readable label that was passed to the boundary. */
  label?: string;
  /** The thrown Error. */
  error: Error;
}

export interface PluginErrorFallbackSpec {
  id: string;
  /** Sort hint — highest priority wins. Built-ins use 0; plugins ≥ 100. */
  priority?: number;
  component: ComponentType<PluginErrorFallbackProps>;
}

// ---------------------------------------------------------------------------
// Tasks — long-running operations visible in the status bar
// ---------------------------------------------------------------------------

/** Handle returned by `host.tasks.start`. All methods are idempotent after a
 *  terminal transition (succeed / fail) — extra calls are no-ops. */
export interface TaskHandle {
  /** Update mid-flight state. `progress` is 0..1 (or null for indeterminate). */
  update: (patch: { progress?: number | null; message?: string | null }) => void;
  /** Mark the task done. The status-bar entry briefly flashes "done" then disappears. */
  succeed: (message?: string) => void;
  /** Mark the task failed. The error surfaces in the status bar; entry disappears after a beat. */
  fail: (error: unknown) => void;
}

export interface TaskStartOptions {
  /** Stable id — defaults to a generated one. Pass an id to allow cross-call updates. */
  id?: string;
  /** One-line label shown in the status bar. */
  label: string;
  /** Optional sub-line shown under the label. */
  message?: string;
  /** 0..1 to start with a determinate bar; omit / null for an indeterminate spinner. */
  progress?: number | null;
}
