import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { providersSettingsPane } from "./application/providersContributions";

const ProvidersPane = lazy(() =>
  import("./ui/ProvidersPane").then(({ ProvidersPane }) => ({ default: ProvidersPane })),
);

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installProviderGateway();
    registerSettingsPane(host, providersSettingsPane(ProvidersPane));
    return disposeGateway;
  },
});
