import { de } from "@/lib/locales/de";
import { definePlugin } from "@/plugins/sdk";

export const localeDe = definePlugin({
  name: "lyra.builtin.locale-de",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("de", de);
    host.i18n.registerLocale({ id: "de", label: "Deutsch", order: 70 });
  },
});
