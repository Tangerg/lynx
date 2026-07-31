import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "@playwright/test";

const VISUAL_URL = "http://127.0.0.1:4174/visual/";
const WCAG_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] as const;

interface FixtureRoute {
  fixture: "agent" | "shell" | "workspace";
  state: string;
  theme?: "light" | "dark";
  motion?: "full";
  fontSize?: number;
}

async function openFixture(page: Page, route: FixtureRoute): Promise<void> {
  const query = new URLSearchParams({
    fixture: route.fixture,
    state: route.state,
    theme: route.theme ?? "light",
  });
  if (route.motion) query.set("motion", route.motion);
  if (route.fontSize !== undefined) query.set("font-size", String(route.fontSize));

  await page.goto(`${VISUAL_URL}?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();

  if (route.fixture === "agent" && route.state === "long-content") {
    await page.locator(".shiki-block .shiki").waitFor();
  }
  if (route.fixture === "shell" && route.state === "populated") {
    await expect(
      page.getByRole("complementary", { name: "Work index" }).getByRole("button", {
        name: "lynx 6",
      }),
    ).toBeVisible();
  }
  if (route.fixture === "workspace" && route.state === "dock-review") {
    await expect(page.locator("[data-diff-file]")).toHaveCount(2);
    await page.locator('[data-diff-file] span[style*="color"]').first().waitFor();
  }
  if (route.fixture === "workspace" && route.state === "settings") {
    await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
  }
}

function pageHorizontalOverflow(page: Page): Promise<number> {
  return page.locator("html").evaluate((element) => element.scrollWidth - element.clientWidth);
}

const ACCESSIBILITY_ROUTES = [
  { fixture: "shell", state: "populated", theme: "light" },
  { fixture: "shell", state: "error", theme: "dark" },
  { fixture: "agent", state: "empty", theme: "light" },
  { fixture: "agent", state: "waiting", theme: "dark" },
  { fixture: "agent", state: "error", theme: "light" },
  { fixture: "agent", state: "long-content", theme: "dark" },
  { fixture: "workspace", state: "dock-review", theme: "light" },
  { fixture: "workspace", state: "dock-error", theme: "dark" },
  { fixture: "workspace", state: "settings", theme: "light" },
] as const satisfies readonly FixtureRoute[];

for (const route of ACCESSIBILITY_ROUTES) {
  test(`WCAG audit ${route.fixture} ${route.state} ${route.theme}`, async ({ page }) => {
    if (route.fixture === "workspace" && route.state === "dock-review") {
      await page.setViewportSize({ width: 1440, height: 900 });
    }
    await openFixture(page, route);

    const results = await new AxeBuilder({ page }).withTags([...WCAG_TAGS]).analyze();
    expect(
      results.violations,
      results.violations
        .map(
          (violation) =>
            `${violation.id}: ${violation.help}\n${violation.nodes
              .map((node) => `  ${node.target.join(" ")}: ${node.failureSummary ?? ""}`)
              .join("\n")}`,
        )
        .join("\n\n"),
    ).toEqual([]);
  });
}

test("motion preference and OS reduced motion share one final authority", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFixture(page, {
    fixture: "shell",
    state: "populated",
    theme: "light",
    motion: "full",
  });

  const drawerGap = page.locator(".agent-drawer-gap");
  await expect(drawerGap).toHaveCSS("transition-duration", "0.3s");

  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(drawerGap).toHaveCSS("transition-duration", "0.001s");

  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFixture(page, { fixture: "shell", state: "populated", theme: "light" });
  await expect(page.locator("html")).toHaveAttribute("data-motion", "off");
  await expect(page.locator(".agent-drawer-gap")).toHaveCSS("transition-duration", "0.001s");
});

test("coarse pointers receive real 44px controls without overlapping hit targets", async ({
  browser,
}) => {
  const { context, page } = await closurePage(browser, {
    hasTouch: true,
    viewport: { width: 1120, height: 720 },
  });
  try {
    await openFixture(page, { fixture: "workspace", state: "dock-light" });
    expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);

    for (const control of [
      page.getByRole("tab", { name: "Plan" }),
      page.getByRole("button", { name: "Hide the context dock" }),
      page.getByRole("button", { name: "Attach image" }),
    ]) {
      const box = await control.boundingBox();
      if (!box) throw new Error("Coarse-pointer control has no layout box");
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }

    await openFixture(page, { fixture: "workspace", state: "settings" });
    const search = page.getByRole("searchbox", { name: "Search settings..." });
    const searchBox = await search.boundingBox();
    if (!searchBox) throw new Error("Settings search has no layout box");
    expect(searchBox.height).toBeGreaterThanOrEqual(44);
    expect(await pageHorizontalOverflow(page)).toBeLessThanOrEqual(0);
  } finally {
    await context.close();
  }
});

test("keyboard-only traversal reaches recovery, HITL, and settings actions", async ({ page }) => {
  await openFixture(page, { fixture: "shell", state: "error", theme: "light" });
  const settings = page.getByRole("button", { name: "Settings" });
  await tabTo(page, settings);
  await assertVisibleKeyboardFocus(settings);

  await openFixture(page, { fixture: "agent", state: "waiting", theme: "dark" });
  const approve = page.getByRole("button", { name: /Approve/ });
  await tabTo(page, approve);
  await assertVisibleKeyboardFocus(approve);
  await page.keyboard.press("Enter");
  await expect(page.getByText("Approved", { exact: true })).toBeVisible();

  await openFixture(page, { fixture: "workspace", state: "settings", theme: "light" });
  const search = page.getByRole("searchbox", { name: "Search settings..." });
  await tabTo(page, search);
  await assertVisibleKeyboardFocus(search);
  await page.keyboard.type("Providers");
  await expect(page.getByRole("heading", { name: "Providers" })).toBeVisible();
});

test("IME composition keeps Enter inside the composer until text is committed", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "steer", theme: "light" });
  const composer = page.getByRole("textbox", { name: "Message composer" });
  await composer.focus();

  await composer.evaluate((element) => {
    const textarea = element as HTMLTextAreaElement;
    textarea.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true, data: "ni" }));
    const setValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
    setValue?.call(textarea, "你");
    textarea.dispatchEvent(
      new InputEvent("input", {
        bubbles: true,
        data: "你",
        inputType: "insertCompositionText",
        isComposing: true,
      }),
    );
    textarea.dispatchEvent(
      new KeyboardEvent("keydown", {
        bubbles: true,
        cancelable: true,
        isComposing: true,
        key: "Enter",
      }),
    );
  });

  await expect(composer).toHaveValue("你");
  await expect(page.locator("html")).not.toHaveAttribute("data-visual-sent-input");
  await composer.evaluate((element) => {
    element.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "你" }));
  });
  await expect(composer).toHaveValue("你");
});

test("message copy writes through the production clipboard path", async ({ context, page }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: "http://127.0.0.1:4174",
  });
  await openFixture(page, {
    fixture: "agent",
    state: "long-content",
    theme: "light",
  });

  const response = page.locator(".msg-content").filter({
    hasText: "The consumer owns persistence policy and transaction scope.",
  });
  await response.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Copy markdown" }).click();

  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("The consumer owns persistence policy and transaction scope.");
});

for (const theme of ["light", "dark"] as const) {
  test(`maximum UI text remains readable without horizontal clipping ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openFixture(page, {
      fixture: "agent",
      state: "long-content",
      theme,
      fontSize: 18,
    });

    await expect(page.locator("body")).toHaveCSS("font-size", "18px");
    expect(await pageHorizontalOverflow(page)).toBeLessThanOrEqual(0);
    await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
    await expect(page).toHaveScreenshot(`closure-${theme}-agent-long-font18-1280x800.png`);

    await openFixture(page, { fixture: "workspace", state: "settings", theme, fontSize: 18 });
    await expect(page.locator("body")).toHaveCSS("font-size", "18px");
    expect(await pageHorizontalOverflow(page)).toBeLessThanOrEqual(0);
    await expect(page.getByRole("searchbox", { name: "Search settings..." })).toBeVisible();
    await expect(page).toHaveScreenshot(`closure-${theme}-settings-font18-1280x800.png`);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`Retina closure ${theme}`, async ({ browser }) => {
    const { context, page } = await closurePage(browser, {
      deviceScaleFactor: 2,
      viewport: { width: 1440, height: 900 },
    });
    try {
      await openFixture(page, { fixture: "agent", state: "waiting", theme });
      expect(await page.evaluate(() => devicePixelRatio)).toBe(2);
      await expect(page).toHaveScreenshot(`closure-${theme}-agent-waiting-retina.png`);

      await openFixture(page, { fixture: "workspace", state: "dock-review", theme });
      await expect(page).toHaveScreenshot(`closure-${theme}-workspace-review-retina.png`);
    } finally {
      await context.close();
    }
  });
}

async function closurePage(
  browser: Browser,
  overrides: {
    deviceScaleFactor?: number;
    hasTouch?: boolean;
    viewport: { width: number; height: number };
  },
) {
  const context = await browser.newContext({
    colorScheme: "light",
    deviceScaleFactor: overrides.deviceScaleFactor ?? 1,
    hasTouch: overrides.hasTouch,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: overrides.viewport,
  });
  return { context, page: await context.newPage() };
}

async function tabTo(page: Page, target: ReturnType<Page["locator"]>, limit = 80): Promise<void> {
  for (let index = 0; index < limit; index += 1) {
    if (await target.evaluate((element) => element === document.activeElement)) return;
    await page.keyboard.press("Tab");
  }
  throw new Error(`Keyboard traversal did not reach ${await target.getAttribute("aria-label")}`);
}

async function assertVisibleKeyboardFocus(target: ReturnType<Page["locator"]>): Promise<void> {
  const style = await target.evaluate((element) => {
    const computed = getComputedStyle(element);
    return {
      backgroundColor: computed.backgroundColor,
      outlineStyle: computed.outlineStyle,
      outlineWidth: computed.outlineWidth,
    };
  });
  expect(
    style.outlineStyle !== "none" ||
      style.outlineWidth !== "0px" ||
      style.backgroundColor !== "rgba(0, 0, 0, 0)",
  ).toBe(true);
}
