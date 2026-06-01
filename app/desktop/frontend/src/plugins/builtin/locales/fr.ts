import { fr } from "@/lib/i18n/locales/fr";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";

export const localeFr = definePlugin({
  name: "lyra.builtin.locale-fr",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("fr", fr);
    host.extensions.contribute(LOCALE, { id: "fr", label: "Français", order: 60 });
  },
});
