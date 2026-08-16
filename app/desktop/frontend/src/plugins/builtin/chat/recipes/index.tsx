import { definePlugin } from "@/plugins/sdk";
import { installRecipeSlashCommands } from "./application/recipeSlashCommands";

export default definePlugin({
  name: "lyra.builtin.recipes-slash",
  setup(ctx) {
    ctx.cleanup(installRecipeSlashCommands(ctx));
  },
});
