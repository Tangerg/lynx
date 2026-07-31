import { createRoot } from "react-dom/client";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/ui";
import { queryClient } from "@/lib/queryClient";
import { setLocale } from "@/lib/i18n";
import { VisualFoundationFixture } from "./VisualFoundationFixture";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";
import { VISUAL_WORK_INDEX_STATES, type VisualWorkIndexState } from "./shellFixtureStates";
import "../src/styles/globals.css";

type FixtureTheme = "light" | "dark";

const VISUAL_NOW = Date.parse("2026-07-31T14:30:00Z");
Date.now = () => VISUAL_NOW;
setLocale("en");

function fixtureTheme(value: string | null): FixtureTheme {
  return value === "dark" ? "dark" : "light";
}

const query = new URLSearchParams(window.location.search);
const theme = fixtureTheme(query.get("theme"));
const sidebarOpen = query.get("sidebar") !== "collapsed";
const requestedFixture = query.get("fixture");
const fixture =
  requestedFixture === "agent" || requestedFixture === "workspace" || requestedFixture === "shell"
    ? requestedFixture
    : "foundation";
const requestedState = query.get("state");
const state: VisualAgentState = VISUAL_AGENT_STATES.includes(requestedState as VisualAgentState)
  ? (requestedState as VisualAgentState)
  : "running";
const workIndexState: VisualWorkIndexState = VISUAL_WORK_INDEX_STATES.includes(
  requestedState as VisualWorkIndexState,
)
  ? (requestedState as VisualWorkIndexState)
  : "populated";
const rootElement = document.documentElement;

rootElement.classList.remove("theme-light", "theme-dark");
rootElement.classList.add(`theme-${theme}`);
rootElement.style.setProperty("--motion-scale", "0");
rootElement.dataset.visualTheme = theme;

const container = document.getElementById("root");
if (!container) throw new Error("Visual fixture root element is missing");

async function fixtureNode(): Promise<ReactNode> {
  if (fixture === "foundation") {
    return <VisualFoundationFixture sidebarOpen={sidebarOpen} />;
  }
  if (fixture === "workspace") {
    const { VisualWorkspaceFixture } = await import("./VisualWorkspaceFixture");
    return <VisualWorkspaceFixture view={query.get("view") === "settings" ? "settings" : "dock"} />;
  }
  if (fixture === "shell") {
    const [{ VisualShellFixture }, { installVisualShellFixture }] = await Promise.all([
      import("./VisualShellFixture"),
      import("./installVisualShellFixture"),
    ]);
    await installVisualShellFixture(workIndexState, theme, sidebarOpen);
    return <VisualShellFixture state={workIndexState} />;
  }
  const [{ VisualAgentStateFixture }, { installVisualAgentFixture }] = await Promise.all([
    import("./VisualAgentStateFixture"),
    import("./installVisualAgentFixture"),
  ]);
  const view = installVisualAgentFixture(state);
  return <VisualAgentStateFixture state={state} theme={theme} view={view} />;
}

const node = await fixtureNode();
createRoot(container).render(
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>{node}</TooltipProvider>
  </QueryClientProvider>,
);

void document.fonts.ready.then(() =>
  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      rootElement.dataset.visualReady = "";
    }),
  ),
);
