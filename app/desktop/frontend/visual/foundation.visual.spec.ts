import { expect, test } from "@playwright/test";

const THEMES = ["light", "dark"] as const;
const SIDEBAR_STATES = ["expanded", "collapsed"] as const;

for (const theme of THEMES) {
  for (const sidebar of SIDEBAR_STATES) {
    test(`foundation ${theme} ${sidebar}`, async ({ page }) => {
      await page.goto(`/visual/?theme=${theme}&sidebar=${sidebar}`);
      await page.locator("html[data-visual-ready]").waitFor();

      const root = page.locator(":root");
      const contentCard = page.getByTestId("content-card");
      const composer = page.getByTestId("composer");

      await expect(root).toHaveClass(new RegExp(`theme-${theme}`));
      await expect(page.getByTestId("sidebar-state")).toHaveText(sidebar);
      await expect(contentCard).toHaveCSS(
        "border-top-left-radius",
        sidebar === "expanded" ? "14.4px" : "0px",
      );
      await expect(composer).toHaveCSS("border-top-width", "1px");
      await expect(page).toHaveScreenshot(`foundation-${theme}-${sidebar}.png`);
    });
  }
}
