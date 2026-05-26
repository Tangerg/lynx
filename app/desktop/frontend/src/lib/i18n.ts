// Thin wrapper over i18next + react-i18next so the rest of the app
// stays on a stable `useT() / setLocale() / useLocale()` API. Each
// locale's dictionary lives in its own file under `lib/locales/`;
// this module just wires them into i18next + exposes the React hooks.
//
// All bundles are statically imported so the entire UI is translated
// from the first paint — locale switches don't trigger a network
// fetch. They're small (~80 keys × ~30 chars each); the whole set is
// well under a KB after gzip.

import i18next from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";
import { de } from "@/lib/locales/de";
import { en } from "@/lib/locales/en";
import { es } from "@/lib/locales/es";
import { fr } from "@/lib/locales/fr";
import { ja } from "@/lib/locales/ja";
import { ko } from "@/lib/locales/ko";
import { zh } from "@/lib/locales/zh";
import { zhTW } from "@/lib/locales/zh-TW";

// Locale ids align with BCP-47 primary subtags plus the one regional
// variant we ship for Traditional Chinese. Add a new locale by
// dropping a `lib/locales/<id>.ts` and wiring it into LOCALES below.
export type Locale = "en" | "zh" | "zh-TW" | "ja" | "ko" | "es" | "fr" | "de";

const STORAGE_KEY = "lyra.locale";

const BUNDLES: Record<Locale, Record<string, string>> = {
  en,
  zh,
  "zh-TW": zhTW,
  ja,
  ko,
  es,
  fr,
  de,
};

const LOCALE_IDS = Object.keys(BUNDLES) as Locale[];

function isLocale(value: string): value is Locale {
  return (LOCALE_IDS as string[]).includes(value);
}

// Pick a starting locale from (a) stored preference, (b) navigator
// language, (c) English. Navigator strings like "zh-CN", "zh-HK",
// "fr-CA" are reduced to the closest shipped bundle.
function detectInitial(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && isLocale(stored)) return stored;
  } catch {
    /* ignore */
  }
  const nav = typeof navigator !== "undefined" ? navigator.language : "";
  return matchNavigator(nav);
}

// Map navigator.language to one of the shipped bundles. Traditional
// Chinese variants (zh-TW, zh-HK, zh-MO) fold into zh-TW; everything
// else "zh-*" lands on simplified zh.
function matchNavigator(nav: string): Locale {
  const low = nav.toLowerCase();
  if (low.startsWith("zh")) {
    return low.includes("tw") || low.includes("hk") || low.includes("mo") ? "zh-TW" : "zh";
  }
  for (const id of LOCALE_IDS) {
    if (id === "en" || id === "zh" || id === "zh-TW") continue;
    if (low.startsWith(id)) return id;
  }
  return "en";
}

const initial = detectInitial();

const resources = Object.fromEntries(
  LOCALE_IDS.map((id) => [id, { translation: BUNDLES[id] }]),
);

void i18next.use(initReactI18next).init({
  resources,
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
// and Intl APIs that read `document.documentElement.lang` (we don't,
// but it's standards-hygiene to keep it in sync).
function syncHtmlLang(loc: Locale): void {
  if (typeof document === "undefined") return;
  document.documentElement.lang =
    loc === "zh" ? "zh-CN" : loc === "zh-TW" ? "zh-TW" : loc;
}
syncHtmlLang(initial);

function getLocale(): Locale {
  const lng = i18next.resolvedLanguage;
  return lng && isLocale(lng) ? lng : "en";
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
  const lng = i18n.resolvedLanguage;
  return lng && isLocale(lng) ? lng : "en";
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

// Native-name labels — what the user sees in the settings picker.
// Native spelling is the convention (Wikipedia, MacOS) so a German
// speaker recognises "Deutsch" without needing to know English.
export const LOCALES: ReadonlyArray<{ id: Locale; label: string }> = [
  { id: "en", label: "English" },
  { id: "zh", label: "简体中文" },
  { id: "zh-TW", label: "繁體中文" },
  { id: "ja", label: "日本語" },
  { id: "ko", label: "한국어" },
  { id: "es", label: "Español" },
  { id: "fr", label: "Français" },
  { id: "de", label: "Deutsch" },
];

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
