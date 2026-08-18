import { expect, test, type Page } from "@playwright/test";
import { en } from "@/lib/i18n/locales/en";
import { DOCK_MIN_WIDTH_PX } from "@/lib/shellGeometry";
import {
  VISUAL_DOCK_WIDTH_PX,
  VISUAL_REVIEW_VIEWPORT,
  VISUAL_WORKSPACE_STATES,
  type VisualWorkspaceState,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";

// Named from the catalogue, not copied out of it. Three literals of this string lived
// here, so changing the copy broke a test that has nothing to do with the copy.
const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };

test.use({ viewport: { width: 1472, height: 900 } });

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
  if (state === "dock-inbox") {
    // Both rows, and the batch count that says one of them holds three asks —
    // the queue is only useful if it distinguishes what is waiting and how much.
    await expect(page.getByText("Which database should the migration target?")).toBeVisible();
    await expect(page.getByText("+2", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-stats") {
    // Every dock view stays MOUNTED, so `.first()` here once matched a hidden
    // tool-stats pane while the diff was the one on screen — and the assertions
    // passed against a view nobody could see. Scope to what is visible.
    const view = page.locator(".agent-workspace-view:visible");
    // Six, since the shells state gained the write whose empty card body started this.
    await expect(view).toContainText("6 calls · 8.6s");
    // The two ways a call fails to deliver, counted apart.
    await expect(view).toContainText("1 failed");
    await expect(view).toContainText("1 denied");
    // Ordered by time SPENT, not by call count: the one 8.4s command has to
    // outrank the faster reads, which is the whole reason this is not a counter.
    const listing = await view.innerText();
    expect(listing.indexOf("shell")).toBeLessThan(listing.indexOf("read"));
    return;
  }
  if (state === "dock-tools") {
    const view = page.locator(".agent-workspace-view:visible");
    // The families, in the table's order and not the runtime's listing order — the
    // fixture reports `shell` first and `search_memory` last, and grouping is the
    // whole feature.
    const listing = await view.innerText();
    expect(listing.indexOf("Shell")).toBeLessThan(listing.indexOf("Files"));
    expect(listing.indexOf("Files")).toBeLessThan(listing.indexOf("Search"));
    // A tool the local family table has never heard of still lists, under the
    // trailing family — the alternative is a call the agent can make that the
    // catalog denies exists.
    await expect(view).toContainText("acme_deploy");
    expect(listing.indexOf("Other")).toBeGreaterThan(listing.indexOf("Recall"));
    return;
  }
  if (state === "dock-empty") {
    await expect(page.getByText("Nothing to compare", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-loading") {
    // Scoped to the tab on screen. Every open tab stays mounted, and a tab that is
    // hidden has its effects torn down — so its query never subscribes and it renders
    // its own busy state indefinitely. Unscoped, this matched three spinners and
    // asserted on the first, which is whichever tab happens to be leftmost.
    await expect(
      page.locator(".agent-context-dock [data-dock-view-id]:visible output[aria-busy=true]"),
    ).toBeVisible();
    return;
  }
  if (state === "dock-error") {
    await expect(page.getByText("Couldn't load the diff", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-file") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("8 lines");
    // The tail of the file's longest line. Whether it can be READ is the clipping
    // check's job; this only pins that the viewer renders the whole line.
    await expect(view).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
    return;
  }
  if (state === "settings") {
    await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
    // The heading is owned by the settings host and renders before the lazy
    // Appearance pane. A pane-owned control is the actual ready boundary for
    // interaction assertions and goldens.
    await expect(page.getByRole("button", { name: en["settings.theme"]! })).toBeVisible();
    return;
  }
  // Exhaustiveness belongs here: an added state must declare its own ready boundary
  // instead of being diagnosed against an unrelated surface.
  throw new Error(`No expectation declared for workspace state "${state}"`);
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
  // Collapsed means there is no destination, rather than a hidden one: the dock
  // is open exactly when the location names a view. What survives is the tab set
  // and the memory of which tab you were on — asserted by the round trip below.
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(page.getByTestId("active-dock-view")).toHaveText("");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,terminal,plan,timeline",
  );
  await page.getByRole("button", { name: "Open right workspace" }).click();

  await expect(page.getByTestId("dock-open")).toHaveText("true");
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");
  await expect(page.getByRole("tab", { name: "Plan" })).toHaveAttribute("data-active", "");
});

test("an unsafe narrow row folds the dock without forgetting its tabs", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await openWorkspace(page, { state: "dock-light" });

  const geometry = await page.locator(".agent-dock-row").evaluate((row) => {
    const conversation = row.firstElementChild;
    const dock = row.querySelector(".agent-context-dock");
    return {
      rowWidth: row.getBoundingClientRect().width,
      conversationWidth: conversation?.getBoundingClientRect().width ?? 0,
      dockVisible: dock ? getComputedStyle(dock).visibility !== "hidden" : false,
    };
  });
  expect(geometry.rowWidth).toBeLessThan(640 + DOCK_MIN_WIDTH_PX);
  expect(geometry.dockVisible).toBe(false);
  expect(geometry.conversationWidth).toBe(geometry.rowWidth);
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(
    page.getByRole("button", { name: "Widen the window to open the right workspace" }),
  ).toBeDisabled();
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,terminal,plan,timeline",
  );
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

  // The panel has to be ON TOP of the dock, not merely mounted. Base UI positions
  // the portaled node with a `transform`, which makes it a stacking context — so
  // the panel's own z-index settles nothing outside it, and with the positioner
  // left at `auto` the whole popup lost to the dock's `z-15` backing and painted
  // entirely behind the panel it was opened from. Every assertion below passed
  // through all of that: the DOM was right and not one pixel was drawn.
  const onTop = await page.locator("[role=combobox]").evaluate((input) => {
    const panel = input.closest("[role=dialog], div[class*='z-50']") ?? input.parentElement!;
    const box = panel.getBoundingClientRect();
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return panel.contains(hit);
  });
  expect(onTop).toBe(true);

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

test("the active overflow tab stays visible and both hidden edges remain signposted", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-light" });

  const strip = page.locator(".agent-dock-tabs");
  await expect(strip).toHaveAttribute("data-overflow-start", "");
  await expect(strip).toHaveAttribute("data-overflow-end", "");
  const [stripBox, activeBox] = await Promise.all([
    strip.boundingBox(),
    page.getByRole("tab", { name: "Plan" }).boundingBox(),
  ]);
  expect(stripBox).not.toBeNull();
  expect(activeBox).not.toBeNull();
  expect(activeBox!.x).toBeGreaterThanOrEqual(stripBox!.x);
  expect(activeBox!.x + activeBox!.width).toBeLessThanOrEqual(stripBox!.x + stripBox!.width);
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
  const settledWidth = String(VISUAL_DOCK_WIDTH_PX - 8);
  await expect(page.getByTestId("persisted-dock-width")).toHaveText(settledWidth);

  await page.getByRole("tab", { name: "Diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
  await expect(page.getByTestId("persisted-dock-width")).toHaveText(settledWidth);

  await page.getByRole("tab", { name: "Plan" }).click();
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
  await expect(page.getByTestId("persisted-dock-width")).toHaveText(settledWidth);
});

// Deliberately NOT the review state: that one is seeded wide enough to exercise
// the diff's split, and this test is about the rail at the general persisted width.
test("dock separator exposes its real range and commits a pointer drag once", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });
  await waitForWorkspaceState(page, "dock-light");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedWidth = page.getByTestId("persisted-dock-width");
  await expect(separator).toHaveAttribute("aria-valuemin", String(DOCK_MIN_WIDTH_PX));
  const max = Number(await separator.getAttribute("aria-valuemax"));
  const now = Number(await separator.getAttribute("aria-valuenow"));
  expect(now).toBe(max);
  await expect(persistedWidth).toHaveText(String(VISUAL_DOCK_WIDTH_PX));

  const dock = page.locator(".agent-context-dock");
  const dockBefore = (await dock.boundingBox())?.width;
  const box = await separator.boundingBox();
  if (!box || dockBefore === undefined) throw new Error("Dock separator has no layout box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 48, box.y + box.height / 2);

  await expect(persistedWidth).toHaveText(String(VISUAL_DOCK_WIDTH_PX));
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "0");
  await expect.poll(async () => (await dock.boundingBox())?.width ?? 0).toBeLessThan(dockBefore);

  await page.mouse.up();
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "1");
  await expect(persistedWidth).not.toHaveText(String(VISUAL_DOCK_WIDTH_PX));
  const settledWidth = await persistedWidth.textContent();
  if (!settledWidth) throw new Error("Persisted dock width is missing");
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
});

test("window clamping does not overwrite the dock preference", async ({ page }) => {
  await page.setViewportSize({ width: 1520, height: 900 });
  await openWorkspace(page, { state: "dock-light" });
  await waitForWorkspaceState(page, "dock-light");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedWidth = page.getByTestId("persisted-dock-width");
  const wideMax = Number(await separator.getAttribute("aria-valuemax"));
  expect(wideMax).toBeGreaterThan(VISUAL_DOCK_WIDTH_PX);
  await expect(separator).toHaveAttribute("aria-valuenow", String(VISUAL_DOCK_WIDTH_PX));
  await expect(persistedWidth).toHaveText(String(VISUAL_DOCK_WIDTH_PX));

  await page.setViewportSize({ width: 1120, height: 720 });
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(separator).toHaveCount(0);
  await expect(persistedWidth).toHaveText(String(VISUAL_DOCK_WIDTH_PX));

  await page.setViewportSize({ width: 1520, height: 900 });
  await page.getByRole("button", { name: "Open right workspace" }).click();
  await expect(separator).toHaveAttribute("aria-valuenow", String(VISUAL_DOCK_WIDTH_PX));
  await expect(persistedWidth).toHaveText(String(VISUAL_DOCK_WIDTH_PX));
});

test("settings filtering and menu dismissal stay inside production semantics", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const search = page.getByRole("searchbox", SETTINGS_SEARCH);
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

  await page.getByRole("searchbox", SETTINGS_SEARCH).fill("Keyboard shortcuts");
  await expect(page.getByRole("heading", { name: "Keyboard shortcuts" })).toHaveCount(1);
  await expect(page.getByText("New session", { exact: true })).toBeVisible();

  await page.getByRole("searchbox", { name: "Filter shortcuts" }).fill("Escape");
  await expect(page.getByText("Close workspace view", { exact: true })).toBeVisible();
  await expect(page.getByText("New session", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Esc", { exact: true })).toBeVisible();
});

test("provider and model settings keep validation local to their form", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });

  await page.getByRole("searchbox", SETTINGS_SEARCH).fill("Providers");
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
        await page.setViewportSize(VISUAL_REVIEW_VIEWPORT);
      }
      await openWorkspace(page, { state, theme });
      await waitForWorkspaceState(page, state);
      // Put the transcript at its resting position before reading the clock —
      // same reason as the agent goldens: stick-to-bottom eases toward a target
      // that `content-visibility` keeps re-measuring, so the same fixture lands
      // a pixel apart between runs and every row in the frame moves with it.
      await page.waitForFunction(() => {
        const scroller = document.querySelector(".msg-scroll-viewport");
        if (!scroller) return true;
        scroller.scrollTop = scroller.scrollHeight;
        const probe = window as unknown as { settle?: { top: number; frames: number } };
        const settle = (probe.settle ??= { top: -1, frames: 0 });
        if (scroller.scrollTop === settle.top) settle.frames += 1;
        else {
          settle.top = scroller.scrollTop;
          settle.frames = 0;
        }
        return settle.frames >= 5;
      });
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
