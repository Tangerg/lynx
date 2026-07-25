import { defineLocale } from "../defineLocale";

export const localeZhTW = defineLocale({
  id: "zh-TW",
  label: "繁體中文",
  order: 20,
  load: () => import("@/lib/i18n/locales/zh-TW").then((m) => m.zhTW),
});
