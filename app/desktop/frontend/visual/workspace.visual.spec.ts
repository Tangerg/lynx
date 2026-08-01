import { expect, test, type Page } from "@playwright/test";
import {
  VISUAL_WORKSPACE_STATES,
  type VisualWorkspaceState,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";

interface WorkspaceRoute {
  state: VisualWorkspaceState;
  theme?: VisualWorkspaceTheme;
}

async function openWorkspace(page: Page, route: WorkspaceRoute): Promise<void> {
  const query = new URLSearchParams({
    fixture: "workspace",
    theme: route.theme ?? "light",
    state: route.state,
  });
  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.getByTestId("workspace-state")).toHaveAttribute("data-state", route.state);
}

async function waitForWorkspaceState(page: Page, state: VisualWorkspaceState): Promise<void> {
  if (state === "dock-light") {
    await expect(page.getByRole("tab", { name: "Plan" })).toHaveAttribute("data-active", "");
    await expect(page.getByText("Task plan", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-review") {
    await expect(page.locator("[data-diff-file]")).toHaveCount(2);
    await page.locator('[data-diff-file] span[style*="color"]').first().waitFor();
    return;
  }
  if (state === "dock-empty") {
    await expect(page.getByText("Nothing to compare", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-loading") {
    await expect(page.locator(".agent-context-dock output[aria-busy=true]")).toBeVisible();
    return;
  }
  if (state === "dock-error") {
    await expect(page.getByText("Couldn't load the diff", { exact: true })).toBeVisible();
    return;
  }
  await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
}

for (const state of VISUAL_WORKSPACE_STATES) {
  test(`production workspace renders ${state}`, async ({ page }) => {
    await openWorkspace(page, { state });
    await waitForWorkspaceState(page, state);
    await expect(page.getByTestId("requested-workspace-state")).toHaveText(state);
  });
}

test("hide and reopen preserve the exact dock view identity", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");

  await page.getByRole("button", { name: "Hide the context dock" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("");
  await page.getByRole("button", { name: "Show the context dock" }).click();

  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");
  await expect(page.getByRole("tab", { name: "Plan" })).toHaveAttribute("data-active", "");
});

test("promote, close, and reopen keep one navigation identity", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("button", { name: "Expand to full width" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("");
  await expect(page.getByTestId("active-main-view")).toHaveText("plan");
  await expect(page.getByText("Plan", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Close" }).click();
  await expect(page.getByTestId("active-main-view")).toHaveText("");
  await page.getByRole("button", { name: "Show the context dock" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");
});

test("dock tabs use roving focus and arrow-key activation", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const plan = page.getByRole("tab", { name: "Plan" });
  await plan.focus();
  await plan.press("ArrowLeft");

  await expect(page.getByTestId("active-dock-view")).toHaveText("terminal");
  await expect(page.getByRole("tab", { name: "Terminal" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Terminal" })).toHaveAttribute("data-active", "");
});

test("file and timeline tabs render through their production view plugins", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "File" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  await expect(page.getByText(/const currentWidth = readDockWidth/)).toBeVisible();

  await page.getByRole("tab", { name: "Timeline" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("timeline");
  await expect(page.getByText("Root run", { exact: true })).toBeVisible();
  await expect(page.getByText("run_root", { exact: true })).toBeVisible();
});

test("light and review views retain independent dock widths", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const separator = page.getByRole("separator", { name: "Resize the context dock" });
  await separator.focus();
  await separator.press("ArrowLeft");
  await expect(page.getByTestId("persisted-light-dock-width")).toHaveText("428");
  await expect(page.getByTestId("persisted-review-dock-width")).toHaveText("720");

  await page.getByRole("tab", { name: "Diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");
  await separator.focus();
  await separator.press("ArrowRight");

  await expect(page.getByTestId("persisted-light-dock-width")).toHaveText("428");
  const reviewWidth = Number(await page.getByTestId("persisted-review-dock-width").textContent());
  expect(reviewWidth).toBeLessThan(720);

  await page.getByRole("tab", { name: "Plan" }).click();
  await expect(page.getByTestId("persisted-light-dock-width")).toHaveText("428");
  await expect(page.getByTestId("persisted-review-dock-width")).toHaveText(String(reviewWidth));
});

test("dock separator exposes its real range and commits a pointer drag once", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const separator = page.getByRole("separator", { name: "Resize the context dock" });
  const persistedWidth = page.getByTestId("persisted-review-dock-width");
  await expect(separator).toHaveAttribute("aria-valuemin", "300");
  const max = Number(await separator.getAttribute("aria-valuemax"));
  const now = Number(await separator.getAttribute("aria-valuenow"));
  expect(now).toBe(max);
  await expect(persistedWidth).toHaveText("720");

  const dock = page.locator(".agent-context-dock");
  const dockBefore = (await dock.boundingBox())?.width;
  const box = await separator.boundingBox();
  if (!box || dockBefore === undefined) throw new Error("Dock separator has no layout box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 48, box.y + box.height / 2);

  await expect(persistedWidth).toHaveText("720");
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "0");
  await expect.poll(async () => (await dock.boundingBox())?.width ?? 0).toBeLessThan(dockBefore);

  await page.mouse.up();
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "1");
  await expect(persistedWidth).not.toHaveText("720");
  const settledWidth = await persistedWidth.textContent();
  if (!settledWidth) throw new Error("Persisted dock width is missing");
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
});

test("window clamping does not overwrite the review preference", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const separator = page.getByRole("separator", { name: "Resize the context dock" });
  const persistedWidth = page.getByTestId("persisted-review-dock-width");
  const wideMax = Number(await separator.getAttribute("aria-valuemax"));
  await expect(separator).toHaveAttribute("aria-valuenow", String(wideMax));
  await expect(persistedWidth).toHaveText("720");

  await page.setViewportSize({ width: 1120, height: 720 });
  await expect
    .poll(async () => Number(await separator.getAttribute("aria-valuemax")))
    .toBeLessThan(wideMax);
  await expect
    .poll(
      async () =>
        Number(await separator.getAttribute("aria-valuenow")) ===
        Number(await separator.getAttribute("aria-valuemax")),
    )
    .toBe(true);
  await expect(persistedWidth).toHaveText("720");

  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(separator).toHaveAttribute("aria-valuenow", String(wideMax));
  await expect(persistedWidth).toHaveText("720");
});

test("settings filtering and menu dismissal stay inside production semantics", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const search = page.getByRole("searchbox", { name: "Search settings..." });
  await search.fill("missing pane");
  await expect(page.getByRole("heading", { name: "Appearance" })).toHaveCount(0);
  await search.fill("Appearance");
  await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();

  const theme = page.getByRole("button", { name: "Theme" });
  await theme.click();
  await expect(page.getByRole("menuitem", { name: "Light" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menuitem", { name: "Light" })).toHaveCount(0);
  await expect(theme).toBeFocused();
});

test("settings hosts shortcut contributions without a second page frame", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });

  await page.getByRole("searchbox", { name: "Search settings..." }).fill("Keyboard shortcuts");
  await expect(page.getByRole("heading", { name: "Keyboard shortcuts" })).toHaveCount(1);
  await expect(page.getByText("Open the command palette", { exact: true })).toBeVisible();

  await page.getByRole("searchbox", { name: "Filter shortcuts" }).fill("Escape");
  await expect(page.getByText("Close workspace view", { exact: true })).toBeVisible();
  await expect(page.getByText("Open the command palette", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Esc", { exact: true })).toBeVisible();
});

test("provider and model settings keep validation local to their form", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });

  await page.getByRole("searchbox", { name: "Search settings..." }).fill("Providers");
  await expect(page.getByRole("heading", { name: "Providers" })).toBeVisible();
  await expect(page.getByText("Utility model", { exact: true })).toBeVisible();
  await expect(page.getByText("Embedding model", { exact: true })).toBeVisible();

  const utilityModel = page.getByRole("button", { name: "Utility model" });
  await utilityModel.click();
  await expect(page.getByRole("menuitem", { name: /GPT-5.6/ })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(utilityModel).toBeFocused();

  const anthropicKey = page.getByLabel("anthropic API key");
  const saveButtons = page.getByRole("button", { name: "Save" });
  await expect(saveButtons.last()).toBeDisabled();
  await anthropicKey.fill("sk-ant-visual");
  await expect(saveButtons.last()).toBeEnabled();
});

test("dock controls expose focusable tooltip help", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const maximize = page.getByRole("button", { name: "Expand to full width" });
  await maximize.hover();
  await expect(page.getByRole("tooltip")).toHaveText("Expand to full width");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).toHaveCount(0);
});

test("dock close control reveals its contextual glyph on hover and focus", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const hide = page.getByRole("button", { name: "Hide the context dock" });
  const swap = hide.locator(".t-icon-swap");
  await expect(swap).toHaveAttribute("data-state", "a");

  await hide.hover();
  await expect(swap).toHaveAttribute("data-state", "b");
  await page.mouse.move(0, 0);
  await expect(swap).toHaveAttribute("data-state", "a");

  await hide.focus();
  await expect(swap).toHaveAttribute("data-state", "b");
});

test("plugin notifications use the production toast and dismiss automatically", async ({
  page,
}) => {
  await openWorkspace(page, { state: "settings" });

  await page.evaluate(() => {
    window.dispatchEvent(
      new CustomEvent("lyra:plugin-toast", {
        detail: { message: "Provider credentials were rejected", level: "error" },
      }),
    );
  });

  const toast = page.locator("[data-sonner-toast]");
  await expect(toast).toContainText("Provider credentials were rejected");
  await expect(toast).toHaveAttribute("data-type", "error");
  await expect.poll(() => toast.count(), { timeout: 6_000 }).toBe(0);
});

test("workspace surfaces do not create page-level horizontal overflow", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const overflow = await page.locator("html").evaluate((element) => {
    return element.scrollWidth - element.clientWidth;
  });
  expect(overflow).toBeLessThanOrEqual(0);
});

for (const theme of ["light", "dark"] as const) {
  for (const state of VISUAL_WORKSPACE_STATES) {
    test(`workspace golden ${theme} ${state}`, async ({ page }) => {
      if (state === "dock-review") {
        await page.setViewportSize({ width: 1440, height: 900 });
      }
      await openWorkspace(page, { state, theme });
      await waitForWorkspaceState(page, state);
      // The light-density fixture deliberately uses a live Running snapshot so
      // the production Plan has useful content. Freeze its elapsed-time label
      // only after bootstrap and initial scroll have settled.
      await page.evaluate(() => {
        const snapshotNow = Date.now();
        Date.now = () => snapshotNow;
      });
      await expect(page).toHaveScreenshot(`workspace-${theme}-${state}.png`);
    });
  }
}
