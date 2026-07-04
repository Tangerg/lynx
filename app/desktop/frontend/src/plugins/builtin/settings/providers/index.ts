import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { providersSettingsPane } from "./application/providersContributions";
import { ProvidersPane } from "./ui/ProvidersPane";

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  version: "1.0.0",
  setup({ host }) {
    installProviderGateway();
    registerSettingsPane(host, providersSettingsPane(ProvidersPane));
  },
});
