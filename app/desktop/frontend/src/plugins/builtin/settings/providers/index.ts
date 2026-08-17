import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { providersSettingsPane } from "./application/providersContributions";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const ProvidersPane = lazy(() =>
  import("./ui/ProvidersPane").then(({ ProvidersPane }) => ({ default: ProvidersPane })),
);

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installProviderGateway();
    let runtimeGeneration = ctx.runtime.runtimeGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.runtimeGeneration();
      if (next === runtimeGeneration) return;
      runtimeGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    registerSettingsPane(ctx, providersSettingsPane(ProvidersPane));
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
