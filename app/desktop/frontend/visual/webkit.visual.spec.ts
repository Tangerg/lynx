import { expect, test, type Page } from "@playwright/test";
import { DOCK_MIN_WIDTH_PX } from "@/lib/shellGeometry";
import { en } from "@/lib/i18n/locales/en";

// Named from the catalogue, not copied out of it. This string had seven literal copies
// across three spec files, so changing one character of the copy broke five tests that
// have nothing to do with the copy.
const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };

test.use({ browserName: "webkit" });

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

  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
}

async function expectNoPageOverflow(page: Page): Promise<void> {
  await expect
    .poll(() =>
      page.locator("html").evaluate((element) => element.scrollWidth - element.clientWidth),
    )
    .toBeLessThanOrEqual(0);
}

test("WebKit shell preserves minimum geometry and drawer focus handoff", async ({ page }) => {
  await openFixture(page, { fixture: "shell", state: "populated", motion: "full" });

  await expect(page.getByRole("complementary", { name: "Work index" })).toBeVisible();
  await expect(page.getByRole("button", { name: "scope 6" })).toBeVisible();
  const drawer = page.locator(".agent-drawer");
  await expect(drawer).toHaveCSS("transition-duration", "0.5s, 0.5s, 0s");
  expect(
    await drawer.evaluate((element) => getComputedStyle(element).transitionTimingFunction),
  ).toContain("linear(");
  await expectNoPageOverflow(page);

  await page.getByRole("button", { name: "Hide sidebar" }).click();
  await expect(page.getByRole("button", { name: "Expand sidebar" })).toBeFocused();
  await page.getByRole("button", { name: "Expand sidebar" }).click();
  await expect(page.getByRole("button", { name: "Hide sidebar" })).toBeFocused();
});

test("WebKit agent HITL remains keyboard-operable", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "waiting", theme: "dark" });

  const approve = page.getByRole("button", { name: /Allow once/ });
  await approve.focus();
  await page.keyboard.press("Enter");

  await expect(page.getByText("Approved", { exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
  await expectNoPageOverflow(page);
});

test("WebKit renders long CJK and highlighted content at maximum UI text size", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openFixture(page, {
    fixture: "agent",
    state: "long-content",
    fontSize: 18,
  });

  await expect(page.locator("body")).toHaveCSS("font-size", "18px");
  await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
  await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
  await expect(page.getByText(/中文混排/)).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
  await expectNoPageOverflow(page);
});

test("WebKit workspace keeps review geometry and separator semantics", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openFixture(page, { fixture: "workspace", state: "dock-review", theme: "dark" });

  await expect(page.locator("[data-diff-file]")).toHaveCount(2);
  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  await expect(separator).toHaveAttribute("aria-valuemin", String(DOCK_MIN_WIDTH_PX));
  await expect
    .poll(async () => Number(await separator.getAttribute("aria-valuenow")))
    .toBeGreaterThan(DOCK_MIN_WIDTH_PX);
  await expectNoPageOverflow(page);
});

test("WebKit settings menu dismisses and returns focus", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openFixture(page, {
    fixture: "workspace",
    state: "settings",
    fontSize: 18,
  });

  const search = page.getByRole("searchbox", SETTINGS_SEARCH);
  await expect(search).toBeVisible();
  const theme = page.getByRole("button", { name: "Theme" });
  await theme.click();
  await expect(page.getByRole("menuitem", { name: "Light" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menuitem", { name: "Light" })).toHaveCount(0);
  await expect(theme).toBeFocused();
  await expectNoPageOverflow(page);
});
