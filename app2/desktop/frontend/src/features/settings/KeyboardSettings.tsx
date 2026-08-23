import { useMemo, useState } from "react";

import { useLocalization } from "../localization/Localization";
import {
  commandCatalog,
  shortcutTokens,
  type CommandScope,
} from "../shell/commandCatalog";

export function KeyboardSettings() {
  const { locale, t } = useLocalization();
  const [query, setQuery] = useState("");
  const commands = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase(locale);
    const collator = new Intl.Collator(locale);
    const rows = commandCatalog
      .map((command) => ({
        command,
        label: t(command.label),
        shortcut: shortcutTokens(command.shortcut),
      }))
      .sort((left, right) => collator.compare(left.label, right.label));
    if (needle === "") return rows;
    return rows.filter(
      (row) =>
        row.label.toLocaleLowerCase(locale).includes(needle) ||
        row.shortcut.join(" ").toLocaleLowerCase(locale).includes(needle),
    );
  }, [locale, query, t]);

  return (
    <section
      className="settings-section keyboard-settings"
      aria-labelledby="keyboard-shortcuts-title"
    >
      <header>
        <div>
          <h2 id="keyboard-shortcuts-title">
            {t("settings.shortcuts.heading")}
          </h2>
          <p>{t("settings.shortcuts.detail")}</p>
        </div>
      </header>
      <label className="keyboard-filter">
        <span aria-hidden="true">⌕</span>
        <span className="sr-only">{t("settings.shortcuts.filterAria")}</span>
        <input
          type="search"
          value={query}
          autoComplete="off"
          placeholder={t("settings.shortcuts.filter")}
          onChange={(event) => setQuery(event.currentTarget.value)}
        />
        {query ? (
          <button
            type="button"
            aria-label={t("settings.shortcuts.clear")}
            onClick={() => setQuery("")}
          >
            ×
          </button>
        ) : null}
      </label>
      {commands.length === 0 ? (
        <p className="keyboard-empty">{t("settings.shortcuts.empty")}</p>
      ) : (
        <div className="keyboard-table-frame">
          <table className="keyboard-table">
            <thead>
              <tr>
                <th>{t("settings.shortcuts.action")}</th>
                <th>{t("settings.shortcuts.scope")}</th>
                <th>{t("settings.shortcuts.shortcut")}</th>
              </tr>
            </thead>
            <tbody>
              {commands.map(({ command, label, shortcut }) => (
                <tr key={command.id}>
                  <td>{label}</td>
                  <td>{t(scopeLabel(command.scope))}</td>
                  <td>
                    <span className="shortcut-keys" dir="ltr">
                      {shortcut.map((token, index) => (
                        <kbd key={`${token}:${index}`}>{token}</kbd>
                      ))}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function scopeLabel(scope: CommandScope) {
  switch (scope) {
    case "global": return "settings.shortcuts.scope.global" as const;
    case "session": return "settings.shortcuts.scope.session" as const;
    case "workspace": return "settings.shortcuts.scope.workspace" as const;
  }
}
