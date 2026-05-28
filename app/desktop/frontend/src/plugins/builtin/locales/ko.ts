import { ko } from "@/lib/i18n/locales/ko";
import { definePlugin } from "@/plugins/sdk";

export const localeKo = definePlugin({
  name: "lyra.builtin.locale-ko",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("ko", ko);
    host.i18n.registerLocale({ id: "ko", label: "한국어", order: 40 });
  },
});
