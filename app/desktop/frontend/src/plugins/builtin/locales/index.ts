// Built-in locale plugins. Each language is its own plugin spec; the
// kernel's `lib/i18n.ts` bootstraps English so first-paint always has
// strings, and these plugins fill in everything else via
// `host.i18n.addBundle` + `host.extensions.contribute(LOCALE, …)`.
//
// Adding a new language is one file + one line in the manifest —
// same as adding a theme. Third-party plugins ship their own locale
// the same way.

export { localeEn } from "./en";
export { localeZh } from "./zh";
export { localeZhTW } from "./zh-TW";
export { localeJa } from "./ja";
export { localeKo } from "./ko";
export { localeEs } from "./es";
export { localeFr } from "./fr";
export { localeDe } from "./de";
