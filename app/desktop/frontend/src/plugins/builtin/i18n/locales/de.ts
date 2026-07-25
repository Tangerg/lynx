import { defineLocale } from "../defineLocale";

export const localeDe = defineLocale({
  id: "de",
  label: "Deutsch",
  order: 70,
  load: () => import("@/lib/i18n/locales/de").then((m) => m.de),
});
