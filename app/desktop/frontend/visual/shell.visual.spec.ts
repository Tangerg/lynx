import { expect, test, type Browser, type Page } from "@playwright/test";
import {
  VISUAL_WORK_INDEX_STATES,
  type VisualShellTheme,
  type VisualWorkIndexState,
} from "./shellFixtureStates";

const VISUAL_URL = "http://127.0.0.1:4174/visual/";

interface ShellRoute {
  theme: VisualShellTheme;
  state: VisualWorkIndexState;
  sidebar?: "expanded" | "collapsed";
}

async function openShell(page: Page, route: ShellRoute): Promise<void> {
  const query = new URLSearchParams({
    fixture: "shell",
    theme: route.theme,
    state: route.state,
    sidebar: route.sidebar ?? "expanded",
  });
  await page.goto(`${VISUAL_URL}?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
}

async function waitForWorkIndexState(page: Page, state: VisualWorkIndexState): Promise<void> {
  const workIndex = page.getByRole("complementary", { name: "Work index" });
  if (state === "populated") {
    await expect(workIndex.getByRole("button", { name: "lynx 6" })).toBeVisible();
  } else if (state === "empty") {
    await expect(workIndex.getByText("No projects", { exact: true })).toBeVisible();
  } else if (state === "loading") {
    await expect(workIndex.locator("output[aria-busy=true]")).toBeVisible();
  } else {
    await expect(workIndex.getByText("Couldn’t load projects", { exact: true })).toBeVisible();
  }
}

function sidebarCssWidth(page: Page): Promise<string> {
  return page
    .locator(".agent-shell")
    .evaluate((element) => getComputedStyle(element).getPropertyValue("--sidebar-width").trim());
}

for (const state of VISUAL_WORK_INDEX_STATES) {
  test(`production Work Index renders ${state}`, async ({ page }) => {
    await openShell(page, { theme: "light", state });
    await waitForWorkIndexState(page, state);
    await expect(page.getByTestId("requested-work-index-state")).toHaveText(state);
  });
}

test("drawer collapse keeps one visible recovery control", async ({ page }) => {
  await openShell(page, { theme: "light", state: "populated" });
  await waitForWorkIndexState(page, "populated");

  const shell = page.locator(".agent-shell");
  await expect(page.getByRole("button", { name: "Hide sidebar" })).toHaveCount(1);
  await page.getByRole("button", { name: "Hide sidebar" }).click();
  await expect(shell).toHaveAttribute("data-sidebar", "collapsed");
  await expect(page.getByRole("button", { name: "Expand sidebar" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Expand sidebar" })).toBeFocused();

  await page.getByRole("button", { name: "Expand sidebar" }).click();
  await expect(shell).toHaveAttribute("data-sidebar", "expanded");
  await expect(page.getByRole("button", { name: "Hide sidebar" })).toHaveCount(1);
  await expect(page.getByRole("button", { name: "Hide sidebar" })).toBeFocused();
});

test("destructive session dialog traps, dismisses, and returns focus", async ({ page }) => {
  await openShell(page, { theme: "light", state: "populated" });
  await waitForWorkIndexState(page, "populated");

  const session = page.getByRole("button", { name: /Refine Runtime protocol/ });
  await session.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Delete" }).click();

  const dialog = page.getByRole("dialog", { name: "Delete this session?" });
  const cancel = dialog.getByRole("button", { name: "Cancel" });
  const remove = dialog.getByRole("button", { name: "Delete" });
  await expect(dialog).toBeVisible();
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(remove).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancel).toBeFocused();

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(session).toBeFocused();

  await session.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Delete" }).click();
  await page.locator('[data-slot="confirm-dialog-backdrop"]').click({ position: { x: 4, y: 4 } });
  await expect(page.getByRole("dialog", { name: "Delete this session?" })).toHaveCount(0);
  await expect(session).toBeFocused();
});

test("resize separator commits once after pointer movement and supports the keyboard", async ({
  page,
}) => {
  await openShell(page, { theme: "light", state: "populated" });
  await waitForWorkIndexState(page, "populated");

  const rail = page.getByRole("separator", { name: "Resize the work index" });
  const persistedWidth = page.getByTestId("persisted-sidebar-width");
  await rail.focus();
  await rail.press("ArrowRight");
  await expect(rail).toHaveAttribute("aria-valuenow", "264");
  await expect(persistedWidth).toHaveText("264");

  const box = await rail.boundingBox();
  if (!box) throw new Error("Resize separator has no layout box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(336, box.y + box.height / 2);
  await expect(persistedWidth).toHaveText("264");
  await expect.poll(() => sidebarCssWidth(page)).toBe("336px");
  await page.mouse.up();
  await expect(persistedWidth).toHaveText("336");
  await expect(rail).toHaveAttribute("aria-valuenow", "336");
});

test("window resize clamps layout without overwriting the persisted preference", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openShell(page, { theme: "light", state: "populated" });
  await waitForWorkIndexState(page, "populated");

  const rail = page.getByRole("separator", { name: "Resize the work index" });
  const persistedWidth = page.getByTestId("persisted-sidebar-width");
  await rail.focus();
  await rail.press("End");
  await expect(persistedWidth).toHaveText("800");
  await expect.poll(() => sidebarCssWidth(page)).toBe("800px");

  await page.setViewportSize({ width: 1120, height: 720 });
  await expect.poll(() => sidebarCssWidth(page)).toBe("480px");
  await expect(rail).toHaveAttribute("aria-valuemax", "480");
  await expect(rail).toHaveAttribute("aria-valuenow", "480");
  await expect(persistedWidth).toHaveText("800");

  await page.setViewportSize({ width: 1440, height: 900 });
  await expect.poll(() => sidebarCssWidth(page)).toBe("800px");
  await expect(rail).toHaveAttribute("aria-valuemax", "800");
  await expect(rail).toHaveAttribute("aria-valuenow", "800");
});

const DPR_ONE_GOLDENS = [
  {
    name: "shell-light-populated-1440x900.png",
    viewport: { width: 1440, height: 900 },
    route: { theme: "light", state: "populated" },
  },
  {
    name: "shell-dark-populated-1440x900.png",
    viewport: { width: 1440, height: 900 },
    route: { theme: "dark", state: "populated" },
  },
  {
    name: "shell-light-loading-1280x800.png",
    viewport: { width: 1280, height: 800 },
    route: { theme: "light", state: "loading" },
  },
  {
    name: "shell-dark-error-1280x800.png",
    viewport: { width: 1280, height: 800 },
    route: { theme: "dark", state: "error" },
  },
  {
    name: "shell-light-collapsed-1120x720.png",
    viewport: { width: 1120, height: 720 },
    route: { theme: "light", state: "populated", sidebar: "collapsed" },
  },
  {
    name: "shell-dark-collapsed-1120x720.png",
    viewport: { width: 1120, height: 720 },
    route: { theme: "dark", state: "populated", sidebar: "collapsed" },
  },
] as const satisfies readonly {
  name: string;
  viewport: { width: number; height: number };
  route: ShellRoute;
}[];

for (const golden of DPR_ONE_GOLDENS) {
  test(`shell golden ${golden.name}`, async ({ page }) => {
    await page.setViewportSize(golden.viewport);
    await openShell(page, golden.route);
    if (golden.route.sidebar !== "collapsed") {
      await waitForWorkIndexState(page, golden.route.state);
    }
    await expect(page).toHaveScreenshot(golden.name);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`shell Retina hairlines ${theme}`, async ({ browser }) => {
    const { context, page } = await retinaPage(browser, theme);
    try {
      await openShell(page, { theme, state: "populated" });
      await waitForWorkIndexState(page, "populated");
      await expect(page).toHaveScreenshot(`shell-${theme}-populated-1440x900-retina.png`);
    } finally {
      await context.close();
    }
  });
}

async function retinaPage(browser: Browser, theme: VisualShellTheme) {
  const context = await browser.newContext({
    colorScheme: theme,
    deviceScaleFactor: 2,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: { width: 1440, height: 900 },
  });
  return { context, page: await context.newPage() };
}
