// Global test setup — runs before every spec.
//
// Each test mutates module-level singletons (Zustand stores, the kernel) so we
// wipe them between tests to keep specs hermetic — one line per store, plus
// tearing down whatever Host a spec stood up.

import { afterEach, beforeEach } from "vitest";
import { MotionGlobalConfig } from "motion/react";
import { useConfigStore } from "@/plugins/sdk/config";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { useNotificationStore } from "@/plugins/sdk/notifications";
import { resetKernelForTest } from "@/plugins/sdk/testKernel";
import { useContextDockStore, WorkspaceFileFocus } from "@/state/contextDockStore";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { installAgentDefaultSessionPort } from "@/plugins/builtin/agent/adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "@/plugins/builtin/agent/adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import {
  getAgentSessionLifecycleSnapshot,
  getActiveSessionId,
  subscribeAgentSessionLifecycle,
  subscribeActiveSessionId,
} from "@/plugins/builtin/agent/public/session";
import type { AgentSessionPorts } from "@/plugins/builtin/agent/public/ports";
import { installComposerStatePorts } from "@/plugins/builtin/chat/composer/adapters/composerStatePorts";
import {
  installRuntimeCapabilityPort,
  resetRuntimeConnectionForTest,
} from "@/plugins/builtin/runtime/adapters/runtimeConnectionProjection";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";

const testAgentSessionPorts: AgentSessionPorts = {
  activeSessionId: getActiveSessionId,
  lifecycleSnapshot: getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeLifecycle: subscribeAgentSessionLifecycle,
};

// Before the ports: installing them subscribes to the location.
configureNavigator(createMemoryNavigator());
installAgentStatePorts();
installAgentDefaultSessionPort();
installAgentRuntimeGateway();
installComposerStatePorts(testAgentSessionPorts);
installWorkspaceNavigationPort();
installRuntimeCapabilityPort();

// Unit tests assert state and accessibility, not wall-clock interpolation.
// Happy DOM does not advance Framer Motion's browser frame loop on teardown,
// so an exit animation can otherwise retain its completion Promise forever.
MotionGlobalConfig.skipAnimations = true;

beforeEach(async () => {
  await resetKernelForTest();
  // A fresh location per spec, with its own history. First, for the same reason.
  configureNavigator(createMemoryNavigator());
  installAgentStatePorts();
  installAgentDefaultSessionPort();
  installAgentRuntimeGateway();
  installComposerStatePorts(testAgentSessionPorts);
  installWorkspaceNavigationPort();
  resetRuntimeConnectionForTest();
  installRuntimeCapabilityPort();
  usePluginErrorStore.setState({ log: [], nextId: 1 });
  useNotificationStore.setState({ log: [], nextId: 1 });
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
  useContextDockStore.setState({
    activeSessionScopeId: null,
    sessionScopes: new Map(),
    dockViewIds: [],
    lastViewId: null,
    fileFocus: WorkspaceFileFocus.empty(),
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
});

afterEach(() => {
  // Clear localStorage so storage specs don't leak between cases.
  try {
    localStorage.clear();
  } catch {
    /* SSR-like envs */
  }
});
