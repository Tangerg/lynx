// Canonical key-combo spelling, applied on both contribute and lookup so a
// registration and a keydown always agree.
//
// Cmd/meta fold to "mod" so registrations are cross-platform by default.
// Unmapped segments pass through unchanged, which keeps a literal "ctrl+k"
// distinct from "mod+k".

const MODIFIER_ALIAS: Record<string, string> = {
  cmd: "mod",
  meta: "mod",
  mod: "mod",
  ctrl: "ctrl",
  control: "ctrl",
  shift: "shift",
  alt: "alt",
  option: "alt",
};

// Matches the common docs convention, e.g. "mod+shift+k".
const MODIFIER_ORDER = ["mod", "ctrl", "alt", "shift"] as const;

/** "Cmd+K" / "cmd+K" / "Mod+k" -> "mod+k". Leftmost segments are modifiers;
 *  the last is the key. */
export function normalizeCombo(combo: string): string {
  const parts = combo.split("+").map((p) => p.trim().toLowerCase());
  const key = parts.pop() ?? "";
  const mods = new Set<string>(parts.map((p) => MODIFIER_ALIAS[p] ?? p));
  const sortedMods = MODIFIER_ORDER.filter((m) => mods.has(m));
  return [...sortedMods, key].join("+");
}
