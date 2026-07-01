// Settings pane: a single discoverability page for every keyboard
// shortcut a plugin has registered. The list is built reactively off the
// `lyra.shortcut` extension point, so plugins that load later automatically
// show up; nothing here knows about specific commands.

import { useMemo, useState } from "react";
import { SHORTCUT, useExtensionPoint } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { splitCombo } from "@/lib/combo";

export function ShortcutsPane() {
  const t = useT();
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
        <div className="text-[16px] font-semibold text-fg">{t("shortcuts.title")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("shortcuts.sub")}</div>
      </div>

      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={t("shortcuts.filter")}
        aria-label={t("shortcuts.filterAria")}
        className="w-full rounded-md border border-line bg-surface-2 px-3 py-2 text-[13px] text-fg placeholder:text-fg-faint outline-none focus-visible:border-line-soft focus-visible:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]"
      />

      <div className="min-h-0 flex-1 overflow-auto rounded-md border border-line bg-surface">
        {filtered.length === 0 ? (
          <div className="px-3 py-6 text-center text-[13px] text-fg-faint">
            {t("shortcuts.empty")}
          </div>
        ) : (
          <table className="w-full border-collapse text-left text-[13px]">
            <thead className="sticky top-0 bg-surface-2 text-[11.5px] font-semibold text-fg-faint">
              <tr>
                <th className="px-3 py-1.5">{t("shortcuts.action")}</th>
                <th className="w-[160px] px-3 py-1.5 text-right">{t("shortcuts.shortcut")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => (
                <tr key={s.key} className="transition-colors hover:bg-surface-2">
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
