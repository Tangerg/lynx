// Capability risk classification (AionUi-style safe / moderate / dangerous).
// Each Host capability is rated by blast radius; `aggregateRisk` rolls a
// plugin's declared set up to its worst level. Used today by the sideload load
// audit log; the Plugins pane risk badge will read it when built (§9.3).

import type { HostCapability } from "./types";

export type CapabilityRisk = "safe" | "moderate" | "dangerous";

// safe      — own data / pure presentation, no outward effect
// moderate  — registers a contribution / changes UI, scoped to this app
// dangerous — reaches the backend/network, or can load code (privilege escalation)
export const CAPABILITY_RISK: Record<HostCapability, CapabilityRisk> = {
  notify: "safe",
  log: "safe",
  i18n: "safe",
  state: "safe",
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
  sidebar: "moderate",
  shortcuts: "moderate",
  agent: "moderate",
  data: "moderate",
  commands: "moderate",
  extensions: "moderate",
  settings: "moderate",
  window: "moderate",
  tasks: "moderate",
  lifecycle: "moderate",
  rpc: "dangerous",
  plugins: "dangerous",
};

const RANK: Record<CapabilityRisk, number> = { safe: 0, moderate: 1, dangerous: 2 };

/** The worst risk level among a plugin's declared capabilities ("safe" if none). */
export function aggregateRisk(capabilities: readonly HostCapability[]): CapabilityRisk {
  let worst: CapabilityRisk = "safe";
  for (const c of capabilities) {
    if (RANK[CAPABILITY_RISK[c]] > RANK[worst]) worst = CAPABILITY_RISK[c];
  }
  return worst;
}
