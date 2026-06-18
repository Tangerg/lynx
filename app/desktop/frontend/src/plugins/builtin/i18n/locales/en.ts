// English locale — registers the picker entry only. The dictionary itself is
// shipped in `lib/i18n/locales/en.ts` and bootstrapped into i18next by
// `lib/i18n.ts` (English is the fallback, so it must be available before any
// plugin runs) — hence no `dict` here.

import { defineLocale } from "../defineLocale";

export const localeEn = defineLocale({ id: "en", label: "English", order: 0 });
