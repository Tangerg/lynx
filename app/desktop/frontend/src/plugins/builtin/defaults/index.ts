import { definePlugin, WINDOW } from "@/plugins/sdk";
import { ACCENT, MESSAGE_ROLE } from "@/plugins/sdk/kernelPoints";
import { DEFAULT_ACCENTS, defaultMessageRoles } from "./application/defaultContributions";
import { PRODUCT_NAME } from "@/product";

export { defaultCommands } from "./commands";
export { defaultDataProviders } from "./dataProviders";

export const defaultTitle = definePlugin({
  name: "scopeapp.builtin.default-title",
  requires: { window: WINDOW },
  setup(ctx) {
    ctx.window.setTitle(PRODUCT_NAME);
  },
});

export const defaultAccents = definePlugin({
  name: "scopeapp.builtin.default-accents",
  setup(ctx) {
    for (const accent of DEFAULT_ACCENTS) {
      ctx.contribute(ACCENT, accent);
    }
  },
});

export const defaultRoles = definePlugin({
  name: "scopeapp.builtin.default-roles",
  setup(ctx) {
    for (const role of defaultMessageRoles()) {
      ctx.contribute(MESSAGE_ROLE, role);
    }
  },
});
