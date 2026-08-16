// Every built-in language, each its own plugin so a third party can ship
// Vietnamese / Arabic / … the same way. English's dictionary is bootstrapped by
// `lib/i18n.ts` for first paint, but its plugin still registers the picker entry.

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
