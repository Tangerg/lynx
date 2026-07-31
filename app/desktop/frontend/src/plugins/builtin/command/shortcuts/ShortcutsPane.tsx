// Settings pane: a single discoverability page for every keyboard
// shortcut a plugin has registered. The list is built reactively off the
// `lyra.shortcut` extension point, so plugins that load later automatically
// show up; nothing here knows about specific commands.

import { useMemo, useState } from "react";
import { Kbd, SearchField } from "@/ui";
import { SHORTCUT, useExtensionPoint } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { splitCombo } from "@/lib/combo";

export function ShortcutsPane() {
  const t = useT();
  const shortcuts = useExtensionPoint(SHORTCUT);
  const [query, setQuery] = useState("");

  // A shortcut's description is a catalog key; resolve it once here so sorting,
  // filtering and rendering all work on the words the user actually sees.
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const rows = shortcuts
      .filter((s) => s.description) // anonymous shortcuts are dev-only noise
      .map((s) => ({ ...s, label: t(s.description ?? "") }))
      .sort((a, b) => a.label.localeCompare(b.label));
    if (!q) return rows;
    return rows.filter((s) => s.label.toLowerCase().includes(q) || s.key.toLowerCase().includes(q));
  }, [shortcuts, query, t]);

  return (
    <div className="flex flex-col gap-3">
      <SearchField
        size="lg"
        value={query}
        onValueChange={setQuery}
        placeholder={t("shortcuts.filter")}
        aria-label={t("shortcuts.filterAria")}
      />

      <div className="min-h-0 flex-1 overflow-auto rounded-lg border-[0.5px] border-field bg-transparent">
        {filtered.length === 0 ? (
          <div className="px-3 py-6 text-center text-ui-lg text-fg-faint">
            {t("shortcuts.empty")}
          </div>
        ) : (
          <table className="w-full border-collapse text-left text-ui-lg">
            <thead className="sticky top-0 bg-surface-2 text-ui-sm font-semibold text-fg-faint">
              <tr>
                <th className="px-3 py-1.5">{t("shortcuts.action")}</th>
                <th className="w-[160px] px-3 py-1.5 text-right">{t("shortcuts.shortcut")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => (
                <tr key={s.key} className="transition-colors hover:bg-hover">
                  <td className="px-3 py-1.5 text-fg">{s.label}</td>
                  <td className="px-3 py-1.5 text-right">
                    <span className="inline-flex items-center gap-1">
                      {splitCombo(s.key).map((part, i) => (
                        <Kbd key={i}>{part}</Kbd>
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
