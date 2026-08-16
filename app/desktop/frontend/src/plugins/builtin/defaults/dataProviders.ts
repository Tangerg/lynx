import { definePlugin } from "@/plugins/sdk";
import { registerDefaultDataProviders } from "./adapters/runtimeDataProviders";

export const defaultDataProviders = definePlugin({
  name: "lyra.builtin.default-data",
  setup(ctx) {
    registerDefaultDataProviders(ctx);
  },
});
