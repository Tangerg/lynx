// Every bundled language is its own plugin. English's dictionary is bootstrapped
// by `lib/i18n.ts` for first paint, but its plugin still registers the picker entry.

import type { AnyPlugin } from "dougong";
import {
  localeDe,
  localeEn,
  localeEs,
  localeFr,
  localeJa,
  localeKo,
  localeZh,
  localeZhTW,
} from "./locales";

export const localePlugins: AnyPlugin[] = [
  localeEn,
  localeZh,
  localeZhTW,
  localeJa,
  localeKo,
  localeEs,
  localeFr,
  localeDe,
];
