import { fr } from "@/lib/i18n/locales/fr";
import { definePlugin } from "@/plugins/sdk";

export const localeFr = definePlugin({
  name: "lyra.builtin.locale-fr",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("fr", fr);
    host.i18n.registerLocale({ id: "fr", label: "Français", order: 60 });
  },
});
