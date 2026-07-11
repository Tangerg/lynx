import { definePlugin } from "@/plugins/sdk";
import { registerDefaultDataProviders } from "./adapters/runtimeDataProviders";

export const defaultData = definePlugin({
  name: "lyra.builtin.default-data",
  version: "1.0.0",
  requires: ["lyra.builtin.runtime"],
  setup({ host }) {
    registerDefaultDataProviders(host);
  },
});
