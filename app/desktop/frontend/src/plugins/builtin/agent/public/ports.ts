// The Agent context's setup-time contract.
//
// Most callers reach these through the module functions beside this file — a
// reference is enough, because nothing invokes it until every plugin is up. A
// plugin that READS them during its own setup has an ordering requirement, and
// under a dependency graph that is a Service. The value is the port surface, so
// declaring the requirement and using it are the same act.

import { service } from "dougong";
import type { AgentSessionLifecycleSnapshot } from "./session";

export interface AgentSessionPorts {
  activeSessionId: () => string;
  lifecycleSnapshot: () => AgentSessionLifecycleSnapshot;
  subscribeActiveSessionId: (listener: (sessionId: string) => void) => () => void;
  subscribeLifecycle: (listener: (state: AgentSessionLifecycleSnapshot) => void) => () => void;
}

export const AGENT_SESSION_PORTS = service<AgentSessionPorts>("scopeapp.agent.sessionPorts");
