import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { ProvidersPane } from "./ui/ProvidersPane";

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  version: "1.0.0",
  setup({ host }) {
    installProviderGateway();
    registerSettingsPane(host, {
      id: "providers",
      label: "settings.pane.providers",
      group: "models",
      icon: "spark",
      order: 50,
      component: ProvidersPane,
    });
  },
});
