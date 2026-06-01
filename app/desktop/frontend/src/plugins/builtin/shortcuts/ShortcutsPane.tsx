// Settings pane: a single discoverability page for every keyboard
// shortcut a plugin has registered. The list is built reactively off the
// `lyra.shortcut` extension point, so plugins that load later automatically
// show up; nothing here knows about specific commands.

import { useMemo, useState } from "react";
import { useShortcuts } from "@/plugins/sdk";

// "Mod+Shift+K" → "⌘ ⇧ K" on Mac, "Ctrl Shift K" elsewhere. Keeps the
// canonical form for matching but presents the platform-native glyphs.
// Detection is one-shot at module load — switching OS mid-session isn't
// a thing.
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

// Named keys whose display form doesn't depend on platform — arrows
// render as glyphs everywhere, "Escape" abbreviates to "Esc".
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

function splitCombo(key: string): string[] {
  return key.split("+").map(formatPart);
}

export function ShortcutsPane() {
  const shortcuts = useShortcuts();
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const rows = shortcuts
      .filter((s) => s.description) // anonymous shortcuts are dev-only noise
      .sort((a, b) => (a.description ?? "").localeCompare(b.description ?? ""));
    if (!q) return rows;
    return rows.filter(
      (s) => (s.description ?? "").toLowerCase().includes(q) || s.key.toLowerCase().includes(q),
    );
  }, [shortcuts, query]);

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <div>
        <div className="text-[15px] font-semibold text-fg">Keyboard shortcuts</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">
          Every keybinding registered by built-in and user plugins. Press a combo anywhere in the
          app to fire it; binding conflicts always resolve to the last registration.
        </div>
      </div>

      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Filter by action or combo…"
        aria-label="Filter shortcuts"
        className="w-full rounded-md border border-line bg-surface-2 px-3 py-2 text-[13px] text-fg placeholder:text-fg-faint outline-none focus-visible:border-line-soft focus-visible:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]"
      />

      <div className="min-h-0 flex-1 overflow-auto rounded-md border border-line bg-surface">
        {filtered.length === 0 ? (
          <div className="px-3 py-6 text-center text-[13px] text-fg-faint">No shortcuts match.</div>
        ) : (
          <table className="w-full border-collapse text-left text-[13px]">
            <thead className="sticky top-0 bg-surface-2 text-[11.5px] font-semibold uppercase tracking-wider text-fg-faint">
              <tr>
                <th className="px-3 py-1.5">Action</th>
                <th className="w-[160px] px-3 py-1.5 text-right">Shortcut</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => (
                <tr
                  key={s.key}
                  className="border-t border-line-soft/50 transition-colors hover:bg-surface-2"
                >
                  <td className="px-3 py-1.5 text-fg">{s.description}</td>
                  <td className="px-3 py-1.5 text-right">
                    <span className="inline-flex items-center gap-1">
                      {splitCombo(s.key).map((part, i) => (
                        <kbd
                          key={i}
                          className="inline-flex min-w-[20px] items-center justify-center rounded-xs border border-line bg-surface-2 px-1.5 py-px font-mono text-[11px] font-semibold text-fg-muted"
                        >
                          {part}
                        </kbd>
                      ))}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
