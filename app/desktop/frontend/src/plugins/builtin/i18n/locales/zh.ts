import { defineLocale } from "../defineLocale";

export const localeZh = defineLocale({
  id: "zh",
  label: "简体中文",
  order: 10,
  load: () => import("@/lib/i18n/locales/zh").then((m) => m.zh),
});
