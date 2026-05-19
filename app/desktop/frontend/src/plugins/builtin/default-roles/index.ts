// Built-in plugin: registers the three default message-role identities
// the AG-UI reducer produces. Plugins can register more (e.g. a
// `developer` role) or override these by registering the same id with a
// different display name / icon.

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.default-roles",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerRole({
      id: "user",
      displayName: "You",
      icon: "user",
      avatarVariant: "msg-user",
    });
    host.message.registerRole({
      id: "assistant",
      displayName: "Sonnet 4.5",
      icon: "spark",
      avatarVariant: "msg-agent",
    });
    host.message.registerRole({
      id: "system",
      displayName: "System",
      icon: "shield",
      avatarVariant: "msg-agent",
    });
  },
});
