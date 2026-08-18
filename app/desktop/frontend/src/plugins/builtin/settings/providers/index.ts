import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { PROVIDERS_PANE } from "../public/panes";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const ProvidersPane = lazy(() =>
  import("./ui/ProvidersPane").then(({ ProvidersPane }) => ({ default: ProvidersPane })),
);

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installProviderGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    registerSettingsPane(ctx, {
      id: PROVIDERS_PANE,
      label: "settings.pane.providers",
      group: "models",
      icon: "spark",
      order: 50,
      component: ProvidersPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
