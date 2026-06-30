type ToastLevel = "info" | "warn" | "error";

// A self-mounting listener (see PluginToaster.tsx) picks up these events and
// renders an animated toast. Keeping the dispatcher event-based means the SDK
// doesn't import React for its notification path.
export const PLUGIN_TOAST_EVENT = "lyra:plugin-toast";

export interface PluginToastDetail {
  message: string;
  level: ToastLevel;
}

export function dispatchToast(message: string, level: ToastLevel): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<PluginToastDetail>(PLUGIN_TOAST_EVENT, { detail: { message, level } }),
  );
}
