import {
  accents,
  themes,
  useShellPreferences,
  type AccentPreference,
  type ThemePreference,
} from "../preferences/ShellPreferences";
import { useLocalization } from "../localization/Localization";
import { isLocale, localeOptions } from "../localization/locales";

export function AppearanceSettings() {
  const preferences = useShellPreferences();
  const { t } = useLocalization();
  return (
    <div className="appearance-settings">
      <section className="settings-section" aria-labelledby="appearance-theme-title">
        <header>
          <div>
            <h2 id="appearance-theme-title">{t("settings.appearance.theme")}</h2>
            <p>{t("settings.appearance.themeDetail")}</p>
          </div>
          <span className="appearance-scheme">{t(themeName(preferences.resolvedTheme))}</span>
        </header>
        <div className="theme-options" role="radiogroup" aria-label={t("settings.appearance.theme")}>
          {themes.map((theme) => (
            <button
              key={theme.id}
              type="button"
              role="radio"
              aria-checked={preferences.theme === theme.id}
              data-theme-preview={theme.id}
              onClick={() => preferences.setTheme(theme.id)}
            >
              <span className="theme-preview" aria-hidden="true">
                <i />
                <i />
                <i />
              </span>
              <strong>{t(themeName(theme.id))}</strong>
              <small>{t(themeDetail(theme.id))}</small>
            </button>
          ))}
        </div>
      </section>
      <section className="settings-section" aria-labelledby="appearance-accent-title">
        <header>
          <div>
            <h2 id="appearance-accent-title">{t("settings.appearance.accent")}</h2>
            <p>{t("settings.appearance.accentDetail")}</p>
          </div>
        </header>
        <div className="accent-options" role="radiogroup" aria-label={t("settings.appearance.accentColor")}>
          {accents.map((accent) => (
            <button
              key={accent.id}
              type="button"
              role="radio"
              aria-checked={preferences.accent === accent.id}
              data-accent-preview={accent.id}
              onClick={() => preferences.setAccent(accent.id)}
            >
              <span aria-hidden="true" />
              {t(accentName(accent.id))}
            </button>
          ))}
        </div>
      </section>
      <section className="settings-section" aria-labelledby="appearance-language-title">
        <header>
          <div>
            <h2 id="appearance-language-title">{t("settings.appearance.language")}</h2>
            <p>{t("settings.appearance.languageDetail")}</p>
          </div>
        </header>
        <label className="appearance-language">
          <span>{t("settings.appearance.language")}</span>
          <select
            value={preferences.locale}
            onChange={(event) => {
              if (isLocale(event.currentTarget.value)) {
                preferences.setLocale(event.currentTarget.value);
              }
            }}
          >
            {localeOptions.map((locale) => (
              <option key={locale.id} value={locale.id} dir={locale.direction}>
                {locale.nativeName}
              </option>
            ))}
          </select>
        </label>
      </section>
    </div>
  );
}

function themeName(theme: ThemePreference) {
  switch (theme) {
    case "system": return "settings.appearance.theme.system.name" as const;
    case "linen": return "settings.appearance.theme.linen.name" as const;
    case "graphite": return "settings.appearance.theme.graphite.name" as const;
  }
}

function themeDetail(theme: ThemePreference) {
  switch (theme) {
    case "system": return "settings.appearance.theme.system.detail" as const;
    case "linen": return "settings.appearance.theme.linen.detail" as const;
    case "graphite": return "settings.appearance.theme.graphite.detail" as const;
  }
}

function accentName(accent: AccentPreference) {
  switch (accent) {
    case "ember": return "settings.appearance.accent.ember" as const;
    case "ocean": return "settings.appearance.accent.ocean" as const;
    case "forest": return "settings.appearance.accent.forest" as const;
    case "violet": return "settings.appearance.accent.violet" as const;
  }
}
