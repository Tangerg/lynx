// Settings pane: a single discoverability page for every keyboard
// shortcut a plugin has registered. The list is built reactively off the
// `lyra.shortcut` extension point, so plugins that load later automatically
// show up; nothing here knows about specific commands.

import { useMemo, useState } from "react";
import { SHORTCUT, useExtensionPoint } from "@/plugins/sdk";
import { splitCombo } from "../comboGlyph";

export function ShortcutsPane() {
  const shortcuts = useExtensionPoint(SHORTCUT);
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
