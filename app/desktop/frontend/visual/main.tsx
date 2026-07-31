import { createRoot } from "react-dom/client";
import type { ReactNode } from "react";
import { TooltipProvider } from "@/ui";
import { VisualFoundationFixture } from "./VisualFoundationFixture";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";
import "../src/styles/globals.css";

type FixtureTheme = "light" | "dark";

function fixtureTheme(value: string | null): FixtureTheme {
  return value === "dark" ? "dark" : "light";
}

const query = new URLSearchParams(window.location.search);
const theme = fixtureTheme(query.get("theme"));
const sidebarOpen = query.get("sidebar") !== "collapsed";
const requestedFixture = query.get("fixture");
const fixture =
  requestedFixture === "agent" || requestedFixture === "workspace"
    ? requestedFixture
    : "foundation";
const requestedState = query.get("state");
const state: VisualAgentState = VISUAL_AGENT_STATES.includes(requestedState as VisualAgentState)
  ? (requestedState as VisualAgentState)
  : "running";
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
  const [{ VisualAgentStateFixture }, { installVisualAgentFixture }] = await Promise.all([
    import("./VisualAgentStateFixture"),
    import("./installVisualAgentFixture"),
  ]);
  const view = installVisualAgentFixture(state);
  return <VisualAgentStateFixture state={state} theme={theme} view={view} />;
}

const node = await fixtureNode();
createRoot(container).render(<TooltipProvider>{node}</TooltipProvider>);

void document.fonts.ready.then(() =>
  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      rootElement.dataset.visualReady = "";
    }),
  ),
);
