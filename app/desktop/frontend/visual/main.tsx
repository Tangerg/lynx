import { createRoot } from "react-dom/client";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { TooltipProvider } from "@/ui";
import { publishMotionScale } from "@/lib/appearance";
import { queryClient } from "@/lib/queryClient";
import { setLocale } from "@/lib/i18n";
import { uiTypeLadderCssVariables } from "@/lib/typography";
import { installDocumentAppearance } from "@/plugins/builtin/theme/adapters/documentAppearance";
import { loadPlugin } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";
import { VisualFoundationFixture } from "./VisualFoundationFixture";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";
import { VISUAL_WORK_INDEX_STATES, type VisualWorkIndexState } from "./shellFixtureStates";
import { isVisualWorkspaceState, type VisualWorkspaceState } from "./workspaceFixtureStates";
import "../src/styles/globals.css";

type FixtureTheme = "light" | "dark";

const VISUAL_NOW = Date.parse("2026-07-31T14:30:00Z");
const VISUAL_CLOCK_STARTED_AT = performance.now();
// Keep time-based labels deterministic without freezing the browser clock.
// Disclosure/scroll libraries use Date.now() to advance their frame loops; a
// constant clock leaves those loops waiting forever and hides the production
// transcript's real initial-scroll behaviour from visual tests.
Date.now = () => VISUAL_NOW + (performance.now() - VISUAL_CLOCK_STARTED_AT);
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
const workspaceState: VisualWorkspaceState = isVisualWorkspaceState(requestedState)
  ? requestedState
  : "dock-review";
const rootElement = document.documentElement;
const motionScale = query.get("motion") === "full" ? 1 : 0;
const requestedFontSize = query.get("font-size");

rootElement.classList.remove("theme-light", "theme-dark");
rootElement.classList.add(`theme-${theme}`);
rootElement.style.setProperty("--motion-scale", String(motionScale));
publishMotionScale(motionScale);
if (motionScale === 0) rootElement.dataset.motion = "off";
else delete rootElement.dataset.motion;
if (requestedFontSize !== null && Number.isFinite(Number(requestedFontSize))) {
  for (const [property, value] of Object.entries(
    uiTypeLadderCssVariables(Number(requestedFontSize)),
  )) {
    rootElement.style.setProperty(property, value);
  }
}
rootElement.dataset.visualTheme = theme;

const container = document.getElementById("root");
if (!container) throw new Error("Visual fixture root element is missing");

async function fixtureNode(): Promise<ReactNode> {
  if (fixture === "foundation") {
    // Even the fixture that renders nothing but shell materials needs the palette
    // and style registered: an unregistered theme id resolves to the dark scheme,
    // and unregistered styles leave the shell on globals.css fallbacks — which is
    // to say, the one fixture named for the foundation would photograph anything
    // but it.
    const [{ default: lyraLight }, { default: lyraDark }, { builtinVisualStyles }] =
      await Promise.all([
        import("@/plugins/builtin/theme/themes/lyra-light"),
        import("@/plugins/builtin/theme/themes/lyra-dark"),
        import("@/plugins/builtin/theme/visualStyles"),
      ]);
    for (const plugin of [lyraLight, lyraDark, ...builtinVisualStyles]) {
      await loadPlugin(plugin);
    }
    return <VisualFoundationFixture sidebarOpen={sidebarOpen} />;
  }
  if (fixture === "workspace") {
    const [{ VisualWorkspaceFixture }, { installVisualWorkspaceFixture }] = await Promise.all([
      import("./VisualWorkspaceFixture"),
      import("./installVisualWorkspaceFixture"),
    ]);
    await installVisualWorkspaceFixture(workspaceState, theme);
    return <VisualWorkspaceFixture state={workspaceState} />;
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
  const view = await installVisualAgentFixture(state);
  return <VisualAgentStateFixture state={state} view={view} />;
}

const node = await fixtureNode();

// Run the real appearance pipeline over the fixture's store. Without this the
// harness photographs globals.css's fallback values: every colour theme and every
// visual style would be registered but never applied, so no palette or material
// regression could reach a screenshot. The store already carries the query
// parameters this file resolved, so the pipeline reproduces the deterministic
// motion and type the specs depend on rather than fighting it.
useUiStore.setState({
  theme,
  motionScale,
  ...(requestedFontSize !== null && Number.isFinite(Number(requestedFontSize))
    ? { fontSize: Number(requestedFontSize) }
    : {}),
});
installDocumentAppearance(useUiStore);

createRoot(container).render(
  <QueryClientProvider client={queryClient}>
    <MotionConfig reducedMotion="user">
      <TooltipProvider>{node}</TooltipProvider>
    </MotionConfig>
  </QueryClientProvider>,
);

void document.fonts.ready.then(() =>
  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      rootElement.dataset.visualReady = "";
    }),
  ),
);
