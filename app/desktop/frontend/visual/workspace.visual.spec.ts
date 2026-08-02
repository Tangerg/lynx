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

test("collapse and reopen preserve the dock workspace", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");

  await page.getByRole("button", { name: "Collapse right workspace" }).click();
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,terminal,plan,timeline",
  );
  await page.getByRole("button", { name: "Open right workspace" }).click();

  await expect(page.getByTestId("dock-open")).toHaveText("true");
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");
  await expect(page.getByRole("tab", { name: "Plan" })).toHaveAttribute("data-active", "");
});

test("closing tabs selects a neighbor without collapsing the workspace", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Plan" }).hover();
  await page.getByRole("button", { name: "Close Plan" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("timeline");
  await expect(page.getByTestId("dock-open")).toHaveText("true");

  await page.getByRole("tab", { name: "Timeline" }).hover();
  await page.getByRole("button", { name: "Close Timeline" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("terminal");
  await expect(page.getByTestId("dock-view-ids")).toHaveText("explorer,file,diff,terminal");
});

test("add-panel menu restores a closed singleton and focuses it", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Terminal" }).hover();
  await page.getByRole("button", { name: "Close Terminal" }).click();
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("terminal");

  await page.getByRole("button", { name: "Browse panels" }).click();
  // The catalog is a searchable combobox, not a menu. Filtering and committing
  // from the keyboard is also the path the control is shaped for: the input takes
  // focus on open and `autoHighlight` puts the first match under Enter.
  await page.getByRole("combobox").fill("Terminal");
  await page.getByRole("option", { name: "Terminal" }).waitFor();
  await page.keyboard.press("Enter");

  await expect(page.getByTestId("active-dock-view")).toHaveText("terminal");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,plan,timeline,terminal",
  );
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
  const fileView = page.locator('[data-dock-view-id="file"]');
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeVisible();

  await page.getByRole("tab", { name: "Timeline" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("timeline");
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeHidden();
  await expect(page.getByText("Root run", { exact: true })).toBeVisible();
  await expect(page.getByText("run_root", { exact: true })).toBeVisible();
});

test("all dock views share one stable user-owned width", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  await separator.focus();
  await separator.press("ArrowRight");
  await expect(page.getByTestId("persisted-dock-width")).toHaveText("424");

  await page.getByRole("tab", { name: "Diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");
  await expect(separator).toHaveAttribute("aria-valuenow", "424");
  await expect(page.getByTestId("persisted-dock-width")).toHaveText("424");

  await page.getByRole("tab", { name: "Plan" }).click();
  await expect(separator).toHaveAttribute("aria-valuenow", "424");
  await expect(page.getByTestId("persisted-dock-width")).toHaveText("424");
});

test("dock separator exposes its real range and commits a pointer drag once", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedWidth = page.getByTestId("persisted-dock-width");
  await expect(separator).toHaveAttribute("aria-valuemin", "300");
  const max = Number(await separator.getAttribute("aria-valuemax"));
  const now = Number(await separator.getAttribute("aria-valuenow"));
  expect(now).toBe(max);
  await expect(persistedWidth).toHaveText("520");

  const dock = page.locator(".agent-context-dock");
  const dockBefore = (await dock.boundingBox())?.width;
  const box = await separator.boundingBox();
  if (!box || dockBefore === undefined) throw new Error("Dock separator has no layout box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 48, box.y + box.height / 2);

  await expect(persistedWidth).toHaveText("520");
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "0");
  await expect.poll(async () => (await dock.boundingBox())?.width ?? 0).toBeLessThan(dockBefore);

  await page.mouse.up();
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "1");
  await expect(persistedWidth).not.toHaveText("520");
  const settledWidth = await persistedWidth.textContent();
  if (!settledWidth) throw new Error("Persisted dock width is missing");
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
});

test("window clamping does not overwrite the dock preference", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedWidth = page.getByTestId("persisted-dock-width");
  const wideMax = Number(await separator.getAttribute("aria-valuemax"));
  expect(wideMax).toBeGreaterThan(520);
  await expect(separator).toHaveAttribute("aria-valuenow", "520");
  await expect(persistedWidth).toHaveText("520");

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
  await expect(persistedWidth).toHaveText("520");

  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(separator).toHaveAttribute("aria-valuenow", "520");
  await expect(persistedWidth).toHaveText("520");
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

test("dock add-panel control names itself and dismisses on Escape", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  // The trigger is an icon with no label beside it, so its own accessible name
  // and native title are the only thing that says what it does.
  const add = page.getByRole("button", { name: "Browse panels" });
  await expect(add).toHaveAttribute("title", "Browse panels");

  await add.click();
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
  await expect(add).toBeFocused();
});

test("dock close control reveals its contextual glyph on hover and focus", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const hide = page.getByRole("button", { name: "Collapse right workspace" });
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
