import { de } from "@/lib/i18n/locales/de";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";

export const localeDe = definePlugin({
  name: "lyra.builtin.locale-de",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("de", de);
    host.extensions.contribute(LOCALE, { id: "de", label: "Deutsch", order: 70 });
  },
});
