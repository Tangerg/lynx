// ShortcutsProvider — one global keydown listener that matches against the
// shortcut registry. Plugins register via `host.shortcuts.register(spec)`;
// this component does the dispatch.
//
// Why a single listener (vs. one per registration):
//   - Browser-level capture: we can preventDefault before React's synthetic
//     event runs.
//   - O(1) lookup by canonical combo — cheaper than running N handlers per
//     keypress and short-circuiting when one matches.
//   - Plugin registration changes (register / unregister) update the
//     registry only; the listener itself never re-binds.

import { useEffect } from "react";
import { lookupExtensionByKey, normalizeCombo, SHORTCUT, usePluginStore } from "@/plugins/sdk";

// Stuff we ignore in form fields by default. `allowInInputs: true` opts in.
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

function comboFromEvent(e: KeyboardEvent): string {
  const parts: string[] = [];
  // `metaKey` is Cmd on Mac. We collapse Cmd + Ctrl into "mod" so a plugin's
  // "Mod+K" registration fires regardless of platform. If a plugin really
  // wants Ctrl-only on Mac, it can register "Ctrl+K" — that path normalizes
  // to "ctrl+k" and only matches when ctrlKey is set without metaKey.
  if (e.metaKey || e.ctrlKey) parts.push("mod");
  if (e.altKey) parts.push("alt");
  if (e.shiftKey) parts.push("shift");
  parts.push(e.key.toLowerCase());
  return normalizeCombo(parts.join("+"));
}

export function ShortcutsProvider() {
  // Subscribe to the extension substrate so the effect tears down + reattaches
  // if the registry changes. (Not strictly necessary — the handler reads
  // through `lookupShortcut` on every keydown — but subscribing keeps the
  // component honest about its dependency.)
  const extensions = usePluginStore((s) => s.extensions);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const combo = comboFromEvent(e);
      const spec = lookupExtensionByKey(SHORTCUT, combo);
      if (!spec) return;
      if (!spec.allowInInputs && isEditableTarget(e.target)) return;
      spec.handler(e);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [extensions]);

  return null;
}
