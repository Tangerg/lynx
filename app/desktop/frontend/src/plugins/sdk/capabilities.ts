// Capability risk classification — each capability rated by blast radius.
//
// Also the grantable vocabulary: sideload discovery authorizes a manifest
// against this table's keys, so a permission nobody rated cannot be granted.

import type { HostCapability } from "./types";

export type CapabilityRisk = "safe" | "moderate" | "dangerous";

// safe      — own data / pure presentation, no outward effect
// moderate  — registers a contribution / changes UI, scoped to this app
// dangerous — reaches the backend/network, or can load code (privilege escalation)
export const CAPABILITY_RISK: Record<HostCapability, CapabilityRisk> = {
  notify: "safe",
  log: "safe",
  i18n: "safe",
  config: "safe",
  storage: "safe",
  theme: "safe",
  tool: "moderate",
  message: "moderate",
  events: "moderate",
  layout: "moderate",
  workspace: "moderate",
  router: "moderate",
  composer: "moderate",
  navigation: "moderate",
  shortcuts: "moderate",
  agent: "moderate",
  data: "moderate",
  commands: "moderate",
  extensions: "moderate",
  settings: "moderate",
  window: "moderate",
  tasks: "moderate",
  lifecycle: "moderate",
  plugins: "dangerous",
};
