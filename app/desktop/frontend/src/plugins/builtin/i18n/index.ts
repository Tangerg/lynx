// The `i18n` package entry: every built-in language shipped as one Plugin Pack.
//
// Each language is still its own plugin (`locales/<lang>.ts`) so a third-party
// can ship Vietnamese / Arabic / … the same way; this pack just bundles the
// first-party set behind a single manifest entry. English isn't strictly needed
// (its dictionary is bootstrapped by `lib/i18n.ts` so first-paint has strings)
// but its plugin still registers the picker entry, so it rides along.

import { definePluginPack } from "@/plugins/sdk";
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

export const localesPack = definePluginPack({
  name: "lyra.builtin.locales",
  version: "1.0.0",
  children: [localeEn, localeZh, localeZhTW, localeJa, localeKo, localeEs, localeFr, localeDe],
});
