import { defineLocale } from "../defineLocale";

export const localeJa = defineLocale({
  id: "ja",
  label: "日本語",
  order: 30,
  load: () => import("@/lib/i18n/locales/ja").then((m) => m.ja),
});
