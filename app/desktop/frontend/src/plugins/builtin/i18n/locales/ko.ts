import { defineLocale } from "../defineLocale";

export const localeKo = defineLocale({
  id: "ko",
  label: "한국어",
  order: 40,
  load: () => import("@/lib/i18n/locales/ko").then((m) => m.ko),
});
