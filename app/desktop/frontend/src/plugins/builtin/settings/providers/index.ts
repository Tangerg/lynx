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
  setup(ctx) {
    const disposeGateway = installProviderGateway();
    registerSettingsPane(ctx, providersSettingsPane(ProvidersPane));
    ctx.cleanup(disposeGateway);
  },
});
