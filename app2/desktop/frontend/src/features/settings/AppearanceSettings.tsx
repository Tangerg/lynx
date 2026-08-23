import {
  accents,
  themes,
  useShellPreferences,
} from "../preferences/ShellPreferences";

export function AppearanceSettings() {
  const preferences = useShellPreferences();
  return (
    <div className="appearance-settings">
      <section className="settings-section" aria-labelledby="appearance-theme-title">
        <header>
          <div>
            <h2 id="appearance-theme-title">Color theme</h2>
            <p>Choose a static Lyra palette or follow the operating system.</p>
          </div>
          <span className="appearance-scheme">{preferences.resolvedTheme}</span>
        </header>
        <div className="theme-options" role="radiogroup" aria-label="Color theme">
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
              <strong>{theme.label}</strong>
              <small>{theme.detail}</small>
            </button>
          ))}
        </div>
      </section>
      <section className="settings-section" aria-labelledby="appearance-accent-title">
        <header>
          <div>
            <h2 id="appearance-accent-title">Accent</h2>
            <p>One functional color for focus, progress, links, and primary actions.</p>
          </div>
        </header>
        <div className="accent-options" role="radiogroup" aria-label="Accent color">
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
              {accent.label}
            </button>
          ))}
        </div>
      </section>
    </div>
  );
}
