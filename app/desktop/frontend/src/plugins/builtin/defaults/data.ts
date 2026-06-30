import { definePlugin } from "@/plugins/sdk";
import { registerDefaultDataProviders } from "./dataProviders";

export const defaultData = definePlugin({
  name: "lyra.builtin.default-data",
  version: "1.0.0",
  setup({ host }) {
    registerDefaultDataProviders(host);
  },
});
