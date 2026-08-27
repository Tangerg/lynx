// The plugin that provides the shell's Services.
//
// One plugin rather than five: the reason any of these changes is the same one —
// the app shell grew a capability — and splitting a tiny provider per
// contract buys indirection, not cohesion. The consumers are still decoupled;
// each declares only the contract it uses, which is the property that matters.

import { definePlugin } from "dougong";
import { addLocaleBundle } from "@/lib/i18n";
import { navigator } from "@/lib/navigation";
import { useContextDockStore } from "@/state/contextDockStore";
import { getConfig, hasConfig, setConfig, useConfigStore } from "./config";
import { WORKSPACE_VIEW } from "./kernelPoints";
import { executeCommand } from "./selectors/commands";
import { lookupExtensionByKey } from "./selectors/extensions";
import {
  COMMANDS,
  CONFIG,
  I18N,
  WINDOW,
  WORKSPACE,
  type CommandsService,
  type ConfigService,
  type I18nService,
  type WindowService,
  type WorkspaceService,
} from "./services";
import { useWindowStore } from "./windowStore";

const config: ConfigService = {
  get: (key, defaultValue) => getConfig(key, defaultValue),
  set: (key, value) => setConfig(key, value),
  has: (key) => hasConfig(key),
  onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
};

const i18n: I18nService = {
  // i18next has no per-key removal, so a bundle is permanent for the session.
  // Safe: `t()` only matters while the contributing plugin's UI is mounted, and a
  // same-name reload overwrites the same keys.
  addBundle: (locale, dict) => addLocaleBundle(locale, dict),
};

const window: WindowService = {
  setTitle: (text) => useWindowStore.getState().setTitle(text),
  setBadge: (n) => useWindowStore.getState().setBadge(Math.max(0, n ?? 0)),
  setWorking: (on) => useWindowStore.getState().setWorking(on),
};

const workspace: WorkspaceService = {
  openView(id) {
    if (!lookupExtensionByKey(WORKSPACE_VIEW, id)) {
      console.warn(`[plugin] workspace.openView("${id}"): no view registered`);
      return;
    }
    navigator().go({ view: id });
  },
  closeView(id) {
    if (navigator().get().view === id) navigator().go({ view: null });
    useContextDockStore.getState().closeDockTab(id);
  },
};

const commands: CommandsService = {
  execute: (id, ...args) => executeCommand(id, ...args),
};

export const shellServices = definePlugin({
  name: "scopeapp.kernel.shell",
  provides: {
    config: CONFIG,
    i18n: I18N,
    window: WINDOW,
    workspace: WORKSPACE,
    commands: COMMANDS,
  },
  setup: () => ({ config, i18n, window, workspace, commands }),
});
