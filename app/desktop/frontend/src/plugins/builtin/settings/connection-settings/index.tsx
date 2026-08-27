import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { CONNECTION_PANE } from "../public/panes";

const ConnectionPane = lazy(() =>
  import("./ui/ConnectionPane").then(({ ConnectionPane }) => ({ default: ConnectionPane })),
);

export default definePlugin({
  name: "scopeapp.builtin.connection-settings",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: CONNECTION_PANE,
      label: "settings.pane.connection",
      group: "general",
      icon: "globe",
      order: 5,
      component: ConnectionPane,
    });
  },
});
