// ShortcutsProvider — one global keydown listener that matches against the
// shortcut registry. Plugins register via `host.shortcuts.register(spec)`;
// this component does the dispatch.
//
// Matching is tinykeys', not ours. Three things it gets right that a
// combo-string comparison against `KeyboardEvent.key` did not:
//   - Letter shortcuts survive a non-US keyboard layout, because the binding
//     names the physical key (see `dispatchBinding`).
//   - `$mod` is Meta on Mac and Control elsewhere, so ⌃K stays with the text
//     field it belongs to instead of opening the palette.
//   - Auto-repeat is one intent: holding ⌘N opens one chat, not thirty.
// Sequences ("Mod+K Mod+S") come with it; nothing registers one yet.
//
// Still a single listener, so a registry change updates one binding map rather
// than N subscriptions, and plugins never touch the DOM to own a chord.

import { useEffect } from "react";
import { tinykeys } from "tinykeys";
import { SHORTCUT, useExtensionPoint } from "@/plugins/sdk";
import { dispatchBinding } from "@/lib/combo";

// `allowInInputs: true` opts in so the shortcut fires even in form fields.
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

export function ShortcutsProvider() {
  const shortcuts = useExtensionPoint(SHORTCUT);

  useEffect(() => {
    const bindings: Record<string, (event: KeyboardEvent) => void> = {};
    for (const spec of shortcuts) {
      // The registry keys this point by canonical combo, so two specs can only
      // collide here if they already collided there — one survived.
      bindings[dispatchBinding(spec.key)] = (event) => {
        if (!spec.allowInInputs && isEditableTarget(event.target)) return;
        spec.handler(event);
      };
    }

    // tinykeys' default `ignore` drops every event whose target is a form
    // control, which is the decision `allowInInputs` exists to make per
    // shortcut — so gate it in the handler above and keep only the two rules
    // that hold for all of them: auto-repeat, and IME composition (a CJK commit
    // keystroke belongs to the input, not to a shortcut).
    return tinykeys(window, bindings, {
      ignore: (event) => event.repeat || event.isComposing,
    });
  }, [shortcuts]);

  return null;
}
