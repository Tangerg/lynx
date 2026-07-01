import { definePlugin } from "@/plugins/sdk";
import { installRecipeSlashCommands } from "./application/recipeSlashCommands";

export default definePlugin({
  name: "lyra.builtin.recipes-slash",
  version: "1.0.0",
  setup({ host }) {
    return installRecipeSlashCommands(host);
  },
});
