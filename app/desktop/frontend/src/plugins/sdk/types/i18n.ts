// Locale registration for the Settings → Language picker.
//
// Each shipped language is its own plugin (`plugins/builtin/locales/
// <id>`) that calls `host.i18n.addBundle(id, dict)` to register the
// translation bundle, and `host.extensions.contribute(LOCALE, spec)` to
// make the language pickable in the UI. A third-party plugin can ship its
// own language the same way — the kernel only bootstraps English.

export interface LocaleSpec {
  /**
   * Fetches this language's dictionary, on demand. A function rather than the
   * dictionary itself: every language ships as its own plugin so a third party
   * can add one, but a statically imported dict puts all of them in the entry
   * payload, which makes that boundary a fiction where it costs the most. The
   * bootstrapped fallback (English) omits this — it is already loaded.
   */
  load?: () => Promise<Record<string, string>>;
  /**
   * BCP-47 (or BCP-47-like) language tag. Used as both the i18next
   * resource key and the `id` Settings → Language writes back to
   * `setLocale()`. Common examples: "en", "zh", "zh-TW", "ja".
   */
  id: string;
  /**
   * Native-name label shown in the picker. Convention is the language's
   * own spelling (Wikipedia / macOS) so speakers recognise it without
   * needing English — "Deutsch" rather than "German", "日本語" rather
   * than "Japanese".
   */
  label: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
}
