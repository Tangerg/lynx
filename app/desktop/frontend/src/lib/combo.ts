// Combo → platform-native display glyphs. "Mod+Shift+K" → ["⌘","⇧","K"] on
// Mac, ["Ctrl","Shift","K"] elsewhere. Keeps the canonical combo for matching
// but presents the keys the way the OS prints them. Detection is one-shot at
// module load — switching OS mid-session isn't a thing.
//
// Pure formatting util shared by the shortcuts pane (one <kbd> per part) and
// the welcome screen. Lives in
// lib/ so any plugin can consume it without reaching into another plugin's
// directory.

const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPod|iPad/.test(navigator.platform);

const MAC_GLYPHS: Record<string, string> = {
  mod: "⌘",
  cmd: "⌘",
  ctrl: "⌃",
  shift: "⇧",
  alt: "⌥",
  option: "⌥",
  meta: "⌘",
};

const PC_LABELS: Record<string, string> = {
  mod: "Ctrl",
  cmd: "Ctrl",
  ctrl: "Ctrl",
  shift: "Shift",
  alt: "Alt",
  option: "Alt",
  meta: "Win",
};

// Named keys whose display form doesn't depend on platform — arrows render as
// glyphs everywhere, "Escape" abbreviates to "Esc".
const NAMED_KEYS: Record<string, string> = {
  escape: "Esc",
  arrowup: "↑",
  arrowdown: "↓",
  arrowleft: "←",
  arrowright: "→",
};

function formatPart(part: string): string {
  const lower = part.toLowerCase();
  const mod = (IS_MAC ? MAC_GLYPHS : PC_LABELS)[lower];
  if (mod) return mod;
  const named = NAMED_KEYS[lower];
  if (named) return named;
  if (lower.length === 1) return lower.toUpperCase();
  // Capitalise multi-char keys (Enter, Tab, Space, …).
  return part.charAt(0).toUpperCase() + part.slice(1).toLowerCase();
}

/** Combo → display parts, e.g. "Mod+Shift+K" → ["⌘","⇧","K"]. */
export function splitCombo(combo: string): string[] {
  return combo.split("+").map(formatPart);
}

/** Combo → compact glyph string, e.g. "Mod+N" → "⌘N". */
export function comboGlyph(combo: string): string {
  return splitCombo(combo).join("");
}

// Canonical modifier → the name tinykeys matches. `$mod` resolves to Meta on
// Mac and Control elsewhere — narrower than "either one, on both platforms",
// which is what the hand-rolled dispatcher did and which made ⌃K on a Mac open
// the command palette. Cocoa has already spent that chord on kill-line.
const DISPATCH_MODIFIERS: Record<string, string> = {
  mod: "$mod",
  cmd: "$mod",
  meta: "$mod",
  ctrl: "Control",
  control: "Control",
  alt: "Alt",
  option: "Alt",
  shift: "Shift",
};

function dispatchKey(key: string): string {
  if (/^[a-z]$/i.test(key)) return `Key${key.toUpperCase()}`;
  if (/^[0-9]$/.test(key)) return `Digit${key}`;
  return key;
}

/**
 * Combo → a tinykeys binding, e.g. "Mod+Shift+K" → "$mod+Shift+KeyK".
 *
 * Letters and digits become PHYSICAL key codes, because a shortcut is a
 * position on the keyboard rather than a character. `KeyboardEvent.key` carries
 * whatever the active layout prints at that position, so matching on it meant
 * every letter shortcut was dead under a Cyrillic, Greek or Dvorak layout —
 * ⌘K there reports `key: "к"`, which no registration was ever looking for.
 *
 * Anything else passes through: tinykeys compares against `KeyboardEvent.key`
 * case-insensitively, so "escape" and "arrowup" already match, and a
 * punctuation key has no code worth guessing at.
 *
 * Space-separated presses are a sequence ("Mod+K Mod+S"), matched with a
 * timeout between presses.
 */
export function dispatchBinding(combo: string): string {
  return combo
    .trim()
    .split(/\s+/)
    .map((press) => {
      const parts = press.split("+").map((part) => part.trim());
      const key = parts.pop() ?? "";
      const modifiers = parts.map((part) => DISPATCH_MODIFIERS[part.toLowerCase()] ?? part);
      return [...modifiers, dispatchKey(key)].join("+");
    })
    .join(" ");
}
