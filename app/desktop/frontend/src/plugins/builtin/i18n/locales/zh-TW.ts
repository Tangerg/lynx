import { zhTW } from "@/lib/i18n/locales/zh-TW";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";

export const localeZhTW = definePlugin({
  name: "lyra.builtin.locale-zh-TW",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("zh-TW", zhTW);
    host.extensions.contribute(LOCALE, { id: "zh-TW", label: "繁體中文", order: 20 });
  },
});
