import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { installRecipeSlashCommands } from "./application/recipeSlashCommands";

export default definePlugin({
  name: "lyra.builtin.recipes-slash",
  requires: { sessions: AGENT_SESSION_PORTS },
  setup(ctx) {
    ctx.cleanup(installRecipeSlashCommands(ctx, ctx.sessions));
  },
});
