import { zhTW } from "@/lib/locales/zh-TW";
import { definePlugin } from "@/plugins/sdk";

export const localeZhTW = definePlugin({
  name: "lyra.builtin.locale-zh-TW",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("zh-TW", zhTW);
    host.i18n.registerLocale({ id: "zh-TW", label: "繁體中文", order: 20 });
  },
});
