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
import { useContextDockStore } from "@/state/contextDockStore";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { installAgentDefaultSessionPort } from "@/plugins/builtin/agent/adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "@/plugins/builtin/agent/adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import { installComposerStatePorts } from "@/plugins/builtin/chat/composer/adapters/composerStatePorts";
import { installRuntimeCapabilityPort } from "@/plugins/builtin/runtime/adapters/runtimeCapabilityStore";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";

// Before the ports: installing them subscribes to the location.
configureNavigator(createMemoryNavigator());
installAgentStatePorts();
installAgentDefaultSessionPort();
installAgentRuntimeGateway();
installComposerStatePorts();
installWorkspaceNavigationPort();
installRuntimeCapabilityPort();

beforeEach(() => {
  // A fresh location per spec, with its own history. First, for the same reason.
  configureNavigator(createMemoryNavigator());
  installAgentStatePorts();
  installAgentDefaultSessionPort();
  installAgentRuntimeGateway();
  installComposerStatePorts();
  installWorkspaceNavigationPort();
  installRuntimeCapabilityPort();
  usePluginStore.getState().resetForTest();
  usePluginErrorStore.setState({ log: [], nextId: 1 });
  useNotificationStore.setState({ log: [], nextId: 1 });
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
  useContextDockStore.setState({
    activeSessionScopeId: "",
    sessionScopes: new Map(),
    dockViewIds: [],
    lastViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
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
