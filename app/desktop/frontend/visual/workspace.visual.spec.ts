import { expect, test } from "@playwright/test";

for (const theme of ["light", "dark"] as const) {
  for (const view of ["dock", "settings"] as const) {
    test(`workspace ${theme} ${view}`, async ({ page }) => {
      await page.goto(`/visual/?fixture=workspace&theme=${theme}&view=${view}`);
      await page.locator("html[data-visual-ready]").waitFor();

      await expect(page.getByTestId(`workspace-${view}`)).toBeVisible();
      await expect(page).toHaveScreenshot(`workspace-${theme}-${view}.png`);
    });
  }
}
