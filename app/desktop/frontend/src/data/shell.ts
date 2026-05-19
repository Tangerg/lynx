// Shell-level data — the things that aren't agent-conversation data: which
// projects/sessions the user has, the working tree, MCP servers.
//
// In a real deployment these would come from app-side Go bindings; here we
// hard-code them so the UI is fully demoable.

import type { SidebarProject, SidebarSession } from "@/components/sidebar/types";

export const SESSIONS: SidebarSession[] = [
  { id: "s1", title: "Refactor auth.ts → Result<T,E>",        status: "running", time: "now",       model: "Sonnet 4.5" },
  { id: "s2", title: "Bug: race in WebSocket reconnect logic", status: "waiting", time: "3m",       model: "Opus 4.1" },
  { id: "s3", title: "Write integration tests for /v2/billing", status: "idle",   time: "1h",       model: "Sonnet 4.5" },
  { id: "s4", title: "Migrate Postgres 15 → 16 on staging",    status: "idle",   time: "yesterday", model: "Opus 4.1" },
  { id: "s5", title: "Draft RFC: query budget enforcement",     status: "idle",   time: "2d",       model: "Sonnet 4.5" },
  { id: "s6", title: "Replace Stripe webhook handler",          status: "idle",   time: "4d",       model: "Haiku 4.5" },
  { id: "s7", title: "Investigate 502s on us-east-1",           status: "idle",   time: "1w",       model: "Sonnet 4.5" },
];

export const PROJECTS: SidebarProject[] = [
  { id: "p1", name: "fern-api",        branch: "feat/result-type", active: true },
  { id: "p2", name: "infra",           branch: "main" },
  { id: "p3", name: "marketing-site",  branch: "main" },
];
