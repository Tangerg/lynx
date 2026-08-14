import { definePlugin } from "@/plugins/sdk";
import { installDesktopWorkingDirectoryPicker } from "./adapters/desktopWorkingDirectoryPicker";

export default definePlugin({
  name: "lyra.builtin.navigation-bootstrap",
  version: "1.0.0",
  setup() {
    return installDesktopWorkingDirectoryPicker();
  },
});
