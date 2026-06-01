// Thin wrapper over i18next + react-i18next so the rest of the app
// stays on a stable `useT() / setLocale() / useLocale()` API.
//
// The kernel ships **only the English bundle** — every other language
// (zh / zh-TW / ja / ko / es / fr / de) is a built-in plugin under
// `plugins/builtin/locales/` that calls `host.i18n.addBundle()` +
// `host.extensions.contribute(LOCALE, …)` in its setup. The picker is driven by
// the plugin store's `locales` registry (read via `useLocales()` from
// the SDK), not a hardcoded array here.
//
// Locale type is `string` rather than a union because the set of
// shipped locales is open: a sideloaded plugin can drop a Vietnamese
// bundle in and the picker shows it. The kernel only knows two things
// statically — what "English" looks like (the bootstrap dict so first
// paint always has strings) and how to detect the user's preferred
// locale from `navigator.language`.

import i18next from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";
import { en } from "@/lib/i18n/locales/en";

export type Locale = string;

const STORAGE_KEY = "lyra.locale";

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
  // Keys are dotted strings ("sidebar.search.label") — treat them as
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
  document.documentElement.lang = loc === "zh" ? "zh-CN" : loc === "zh-TW" ? "zh-TW" : loc;
}
syncHtmlLang(initial);

function getLocale(): Locale {
  return i18next.resolvedLanguage ?? "en";
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

/** Reactive locale hook — components using this re-render on change. */
export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return i18n.resolvedLanguage ?? "en";
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
