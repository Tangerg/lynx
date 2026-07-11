import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { ACCENT, MESSAGE_ROLE } from "@/plugins/sdk/kernelPoints";
import { DEFAULT_ACCENTS, defaultMessageRoles } from "./application/defaultContributions";

export { defaultCommands } from "./commands";
export { defaultData } from "./data";

export const defaultTitle = definePlugin({
  name: "lyra.builtin.default-title",
  version: "1.0.0",
  setup({ host }) {
    host.window.setTitle("Lyra");
  },
});

export const defaultAccents = definePlugin({
  name: "lyra.builtin.default-accents",
  version: "1.0.0",
  setup({ host }) {
    for (const accent of DEFAULT_ACCENTS) {
      host.extensions.contribute(ACCENT, accent);
    }
  },
});

export const defaultRoles = definePlugin({
  name: "lyra.builtin.default-roles",
  version: "1.0.0",
  setup({ host }) {
    for (const role of defaultMessageRoles(t)) {
      host.extensions.contribute(MESSAGE_ROLE, role);
    }
  },
});
