import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { connectionSettingsPane } from "./application/connectionContributions";

const ConnectionPane = lazy(() =>
  import("./ui/ConnectionPane").then(({ ConnectionPane }) => ({ default: ConnectionPane })),
);

export default definePlugin({
  name: "lyra.builtin.connection-settings",
  setup(ctx) {
    registerSettingsPane(ctx, connectionSettingsPane(ConnectionPane));
  },
});
