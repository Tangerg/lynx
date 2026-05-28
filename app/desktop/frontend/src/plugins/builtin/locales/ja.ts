import { ja } from "@/lib/i18n/locales/ja";
import { definePlugin } from "@/plugins/sdk";

export const localeJa = definePlugin({
  name: "lyra.builtin.locale-ja",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("ja", ja);
    host.i18n.registerLocale({ id: "ja", label: "日本語", order: 30 });
  },
});
