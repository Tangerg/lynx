import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ProvidersPane } from "./ProvidersPane";

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "providers",
      label: "settings.pane.providers",
      group: "models",
      icon: "spark",
      order: 50,
      component: ProvidersPane,
    });
  },
});
