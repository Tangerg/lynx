import { zh } from "@/lib/i18n/locales/zh";
import { definePlugin } from "@/plugins/sdk";

export const localeZh = definePlugin({
  name: "lyra.builtin.locale-zh",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("zh", zh);
    host.i18n.registerLocale({ id: "zh", label: "简体中文", order: 10 });
  },
});
