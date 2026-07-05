import { definePlugin } from "@/plugins/sdk";
import { welcomeEmptySlot } from "./application/welcomeContributions";
import { WelcomeScreen } from "./ui/WelcomeScreen";

export default definePlugin({
  name: "lyra.builtin.welcome-screen",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.empty", welcomeEmptySlot(WelcomeScreen));
  },
});
