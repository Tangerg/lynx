// Combo → platform-native display glyphs. "Mod+Shift+K" → ["⌘","⇧","K"] on Mac,
// ["Ctrl","Shift","K"] elsewhere. Keeps the canonical combo for matching but
// presents the keys the way the OS prints them. Detection is one-shot at module
// load — switching OS mid-session isn't a thing.
//
// Shared by the command palette (compact glyph string) and the shortcuts pane
// (one <kbd> per part), so a command's combo renders identically in both.

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
