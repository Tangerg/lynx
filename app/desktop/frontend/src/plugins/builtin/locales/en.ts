// English locale plugin — registers the picker entry. The dictionary
// itself is shipped in `lib/locales/en.ts` and bootstrapped into
// i18next by `lib/i18n.ts` (English is the fallback, so it must be
// available before any plugin runs).

import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";

export const localeEn = definePlugin({
  name: "lyra.builtin.locale-en",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(LOCALE, { id: "en", label: "English", order: 0 });
  },
});
