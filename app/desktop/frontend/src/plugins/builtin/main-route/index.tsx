// Built-in plugin: registers the default "/" route → AgentClientPage.
//
// Pulled out of router.tsx so the router itself stays inert: the page
// that lives at "/" is now a (replaceable) plugin contribution. A user
// plugin could register a different "/" route to swap out the main UI
// entirely, or contribute additional routes like "/runs/$runId".

import { AgentClientPage } from "@/pages/AgentClientPage";
import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.main-route",
  version: "1.0.0",
  setup({ host }) {
    host.router.register({
      id: "main",
      path: "/",
      component: AgentClientPage,
      order: 0,
    });
  },
});
