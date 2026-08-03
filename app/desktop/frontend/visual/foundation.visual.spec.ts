import { expect, test, type Page } from "@playwright/test";

const THEMES = ["light", "dark"] as const;
const SIDEBAR_STATES = ["expanded", "collapsed"] as const;

/** The pixel value `--app-content-card-radius` resolves to, measured by letting the
 *  engine compute it on a throwaway element. Reading the custom property directly
 *  returns its unevaluated `calc()`, which is not what a radius assertion can
 *  compare against. */
function declaredCardRadius(page: Page): Promise<string> {
  return page.evaluate(() => {
    const probe = document.createElement("div");
    probe.style.borderTopLeftRadius = "var(--app-content-card-radius)";
    document.body.append(probe);
    const radius = getComputedStyle(probe).borderTopLeftRadius;
    probe.remove();
    return radius;
  });
}

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
      // Resolved from the token rather than written as a number: the corner is the
      // active visual style's to declare, and a literal here would assert one
      // style's shape against every other one. What the shell owns — and what this
      // guards — is that the card squares off with nothing left to be rounded
      // against.
      await expect(contentCard).toHaveCSS(
        "border-top-left-radius",
        sidebar === "expanded" ? await declaredCardRadius(page) : "0px",
      );
      // No border: the composer's edge is the ring in its own box-shadow, asserted
      // in full where the rest of its material is. What belongs here is only that
      // the foundation fixture builds the real surface and not a stand-in.
      await expect(composer).toHaveCSS("border-top-width", "0px");
      await expect(composer).not.toHaveCSS("box-shadow", "none");
      await expect(page).toHaveScreenshot(`foundation-${theme}-${sidebar}.png`);
    });
  }
}
