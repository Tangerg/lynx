// Built-in plugin: registers the default "/" route → AgentClientPage.
//
// Pulled out of router.tsx so the router itself stays inert: the page
// that lives at "/" is now a (replaceable) plugin contribution. A user
// plugin could register a different "/" route to swap out the main UI
// entirely, or contribute additional routes like "/runs/$runId".

import { AgentClientPage } from "@/pages/AgentClientPage";
import { definePlugin } from "@/plugins/sdk";
import { ROUTE } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "scopeapp.builtin.main-route",
  setup(ctx) {
    ctx.contribute(ROUTE, { id: "main", path: "/", order: 0, component: AgentClientPage });
  },
});
