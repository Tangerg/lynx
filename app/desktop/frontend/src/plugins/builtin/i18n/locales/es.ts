import { defineLocale } from "../defineLocale";

export const localeEs = defineLocale({
  id: "es",
  label: "Español",
  order: 50,
  load: () => import("@/lib/i18n/locales/es").then((m) => m.es),
});
