// Thin wrapper over i18next + react-i18next so the rest of the app
// stays on a stable `useT() / setLocale() / useLocale()` API.
//
// The kernel ships **only the English bundle** — every other language
// (zh / zh-TW / ja / ko / es / fr / de) is a built-in plugin under
// `plugins/builtin/locales/` that calls `host.i18n.addBundle()` +
// `host.extensions.contribute(LOCALE, …)` in its setup. The picker is driven by
// the plugin store's `locales` registry (read via `useExtensionPoint(LOCALE)` from
// the SDK), not a hardcoded array here.
//
// Locale type stays `string` because selection and browser preference are
// runtime values. The kernel knows statically what "English" looks like (the
// bootstrap dict so first paint always has strings) and how to detect the
// user's preferred locale from `navigator.language`.

import i18next from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";

/**
 * A translated sentence that contains markup — `<code>`, `<strong>`, a link.
 *
 * Re-exported here so the whole app has one i18n import. The alternative that
 * had grown instead was splitting such a sentence into fragments around the JSX
 * ("… containing", "into", "and restarting the app"), which cannot be reordered
 * by a translator and so is not translatable at all.
 */
export { Trans } from "react-i18next";
import { en } from "@/lib/i18n/locales/en";

export type Locale = string;

const STORAGE_KEY = "scopeapp.locale";

function detectInitial(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return stored;
  } catch {
    /* ignore */
  }
  const nav = typeof navigator !== "undefined" ? navigator.language : "";
  // Fold zh-* variants to the simplified / traditional split here; the
  // locale plugin loader (later) tolerates either "zh" or "zh-CN".
  const low = nav.toLowerCase();
  if (low.startsWith("zh")) {
    return low.includes("tw") || low.includes("hk") || low.includes("mo") ? "zh-TW" : "zh";
  }
  // For everything else, hand i18next the primary subtag — it'll fall
  // back to English if the matching plugin hasn't registered (yet).
  return low.split("-")[0] || "en";
}

const initial = detectInitial();

void i18next.use(initReactI18next).init({
  // Only English is bootstrapped — locale plugins add the rest at
  // plugin-setup time via `addLocaleBundle()`.
  resources: { en: { translation: en } },
  lng: initial,
  fallbackLng: "en",
  // Keys are dotted strings ("sidebar.action.newSession") — treat them as
  // literal, not as nested paths.
  keySeparator: false,
  nsSeparator: false,
  interpolation: { escapeValue: false },
  returnNull: false,
});

// `lang` attribute on <html> drives browser-side a11y, font selection,
// and Intl APIs that read `document.documentElement.lang`.
function syncHtmlLang(loc: Locale): void {
  if (typeof document === "undefined") return;
  // Only "zh" needs remapping to the explicit "zh-CN" region; every other
  // locale (incl. "zh-TW") already equals its lang attribute.
  document.documentElement.lang = loc === "zh" ? "zh-CN" : loc;
}
syncHtmlLang(initial);

function getLocale(): Locale {
  // `language` is the requested locale identity; `resolvedLanguage` may be the
  // English fallback while that locale's lazy plugin has not loaded yet. Using
  // the fallback here made cold-start setup believe the user had selected
  // English, so it never loaded the requested dictionary. Keep selection
  // identity separate from resource-resolution fallback.
  return i18next.language ?? i18next.resolvedLanguage ?? "en";
}

/** The active language tag, read outside React (plugin setup, bootstrap). */
export function activeLocale(): Locale {
  return getLocale();
}

export function setLocale(loc: Locale): void {
  if (loc === getLocale()) return;
  void i18next.changeLanguage(loc);
  try {
    localStorage.setItem(STORAGE_KEY, loc);
  } catch {
    /* ignore */
  }
  syncHtmlLang(loc);
}

export function t(key: string, params?: Record<string, string | number>): string {
  return i18next.t(key, params) as string;
}

/**
 * A translator, as the contribution factories take one.
 *
 * Nine modules had each declared this locally — and they had already drifted:
 * eight spelled `(key) => string` while one carried interpolation params, so the
 * same contract existed in two shapes. It is `typeof t` because that is what it
 * always was: the function these factories are handed. A caller that only reads
 * keys still satisfies it — fewer parameters is assignable.
 */
export type Translate = typeof t;

/** Reactive locale hook — components using this re-render on change. */
export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return i18n.language ?? i18n.resolvedLanguage ?? "en";
}

/** Hook returning a translate fn bound to the live locale. The returned
 *  reference is stable across renders (until the language changes) so it's
 *  safe to use in `useMemo` / `useCallback` deps. */
export function useT(): typeof t {
  // Subscribe for re-renders on language change; the module-level `t`
  // reads i18next live so it always sees the new locale.
  useTranslation();
  return t;
}

/**
 * Merge `dict` into the dictionary for `locale`. Existing keys are
 * overwritten; new keys land alongside the kernel's strings. Used by
 * `host.i18n.addBundle` so plugins can contribute their own labels.
 *
 * i18next has no public per-key removal, so plugin unload doesn't roll
 * the bundle back. In practice this is fine — keys are unreferenced
 * after the plugin's UI is gone, and a same-name reload overwrites
 * cleanly.
 */
export function addLocaleBundle(locale: string, dict: Record<string, string>): void {
  i18next.addResourceBundle(locale, "translation", dict, true, true);
}
