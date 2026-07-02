// OS-level desktop notification — the webview's Notification API, distinct from
// lib/notify (the in-app toast + feed). Used to alert the user that a run
// finished / needs input WHILE the app window is unfocused. The CALLER owns the
// focus gate: never steal attention while the user is watching the run; only
// fire when the window is blurred or hidden (the universal pattern across
// Claude Code / codex / opencode / Kimi / crush — notify only when tabbed away).
//
// Permission is primed once via ensureOsNotifyPermission (called at load, while
// the window is focused so the prompt is allowed). Unsupported / denied → no-op.

// ensureOsNotifyPermission requests notification permission once, lazily. Call
// it early (plugin setup, app focused) so the browser allows the prompt — a
// request issued from an unfocused window is often silently dropped.
export function ensureOsNotifyPermission(): void {
  if (typeof Notification === "undefined") return;
  if (Notification.permission === "default") void Notification.requestPermission();
}

interface OsNotifyOptions {
  body?: string;
  // tag coalesces repeats: a second notification with the same tag replaces the
  // first rather than stacking (e.g. per-session, so a busy session pings once).
  tag?: string;
}

// osNotify fires one desktop notification when permission is granted; otherwise
// a no-op. Clicking it refocuses the app window.
export function osNotify(title: string, opts?: OsNotifyOptions): void {
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  try {
    const n = new Notification(title, { body: opts?.body, tag: opts?.tag });
    n.onclick = () => {
      window.focus();
      n.close();
    };
  } catch {
    // Some webviews reject Notification construction without a service worker —
    // there's nothing actionable, so skip silently.
  }
}
