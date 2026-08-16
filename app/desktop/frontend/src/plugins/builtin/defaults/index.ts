import { definePlugin, WINDOW } from "@/plugins/sdk";
import { ACCENT, MESSAGE_ROLE } from "@/plugins/sdk/kernelPoints";
import { DEFAULT_ACCENTS, defaultMessageRoles } from "./application/defaultContributions";

export { defaultCommands } from "./commands";
export { defaultDataProviders } from "./dataProviders";

export const defaultTitle = definePlugin({
  name: "lyra.builtin.default-title",
  requires: { window: WINDOW },
  setup(ctx) {
    ctx.window.setTitle("Lyra");
  },
});

export const defaultAccents = definePlugin({
  name: "lyra.builtin.default-accents",
  setup(ctx) {
    for (const accent of DEFAULT_ACCENTS) {
      ctx.contribute(ACCENT, accent);
    }
  },
});

export const defaultRoles = definePlugin({
  name: "lyra.builtin.default-roles",
  setup(ctx) {
    for (const role of defaultMessageRoles()) {
      ctx.contribute(MESSAGE_ROLE, role);
    }
  },
});
