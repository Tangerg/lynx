import { es } from "@/lib/locales/es";
import { definePlugin } from "@/plugins/sdk";

export const localeEs = definePlugin({
  name: "lyra.builtin.locale-es",
  version: "1.0.0",
  setup({ host }) {
    host.i18n.addBundle("es", es);
    host.i18n.registerLocale({ id: "es", label: "Español", order: 50 });
  },
});
