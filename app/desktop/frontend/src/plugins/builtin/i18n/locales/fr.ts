import { defineLocale } from "../defineLocale";

export const localeFr = defineLocale({
  id: "fr",
  label: "Français",
  order: 60,
  load: () => import("@/lib/i18n/locales/fr").then((m) => m.fr),
});
