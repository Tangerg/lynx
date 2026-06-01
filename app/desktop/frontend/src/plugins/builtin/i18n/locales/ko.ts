import { ko } from "@/lib/i18n/locales/ko";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";

export const localeKo = definePlugin({
  name: "lyra.builtin.locale-ko",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("ko", ko);
    host.extensions.contribute(LOCALE, { id: "ko", label: "한국어", order: 40 });
  },
});
