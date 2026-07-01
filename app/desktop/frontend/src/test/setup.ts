// Global test setup — runs before every spec.
//
// Each test mutates module-level singletons (Zustand stores, plugin
// registry) so we wipe them between tests to keep specs hermetic. New
// registry slots only need to be added in `registry.INITIAL_STATE`; this
// file stays a one-liner per store.

import { afterEach, beforeEach } from "vitest";
import { useConfigStore } from "@/plugins/sdk/config";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { useNotificationStore } from "@/plugins/sdk/notifications";
import { usePluginStore } from "@/plugins/sdk/registry";
import { _resetAllSlices } from "@/plugins/sdk/stateSlice";
import { installAgentStatePorts } from "@/plugins/builtin/agent/public/statePorts";

installAgentStatePorts();

beforeEach(() => {
  installAgentStatePorts();
  usePluginStore.getState().resetForTest();
  usePluginErrorStore.setState({ log: [], nextId: 1 });
  useNotificationStore.setState({ log: [], nextId: 1 });
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
  _resetAllSlices();
});

afterEach(() => {
  // Clear localStorage so storage specs don't leak between cases.
  try {
    localStorage.clear();
  } catch {
    /* SSR-like envs */
  }
});
