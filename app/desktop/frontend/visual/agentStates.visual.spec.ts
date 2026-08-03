import { expect, test } from "@playwright/test";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";

const EXPECTED_ATTENTION: Record<VisualAgentState, string> = {
  empty: "idle",
  idle: "finished",
  running: "running",
  steer: "running",
  waiting: "waiting",
  question: "waiting",
  terminal: "finished",
  canceled: "finished",
  error: "finished",
  recovery: "finished",
  delegated: "running",
  "long-content": "finished",
};

for (const state of VISUAL_AGENT_STATES) {
  test(`canonical agent projection renders ${state}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const fixture = page.getByTestId("agent-state");
    await expect(fixture).toHaveAttribute("data-state", state);
    await expect(fixture).toHaveAttribute("data-attention", EXPECTED_ATTENTION[state]);
  });
}

test("HITL approval settles through the exact Run and Item identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /Approve/ }).click();

  await expect(page.getByText("Approved", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
});

test("HITL rejection preserves the same exact interrupt identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /Decline/ }).click();

  await expect(page.getByText("Declined", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
});

test("question settlement uses the exact interrupt identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /Race detector/ }).click();
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page.getByText("Answered", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_question");
});

test("delegated cancellation targets the selected child Run", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Cancel this run" }).first().click();

  await expect(page.locator("html")).toHaveAttribute("data-visual-canceled-run", "run_child");
});

test("delegated narrative stays under its exact spawning Item anchor", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated");
  await page.locator("html[data-visual-ready]").waitFor();

  const spawningItem = page.locator("#item_delegate");
  await expect(spawningItem).toHaveCount(1);
  await expect(spawningItem.getByRole("button", { name: /Sub-agent/ }).first()).toBeVisible();
});

test("running composer exposes both steer and stop actions without unnamed controls", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=steer");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("button", { name: "Steer the running turn" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop (Esc)" })).toBeVisible();

  await page.getByRole("button", { name: "Stop (Esc)" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-visual-stopped-root", "run_root");

  await page.getByRole("button", { name: "Steer the running turn" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-visual-steered-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-steered-segment", "seg_root");
  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-sent-input",
    /Tighten the error copy and continue/,
  );
  await expect(page.getByRole("textbox", { name: "Message composer" })).toHaveValue("");
});

test("plan disclosure expands through the production banner contribution", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Expand plan (1/3 · 33%)" }).click();

  await expect(page.getByText("Run conformance gates", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Collapse plan list" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
});

test("composer keeps one production edge and 6/8 footer inset", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=empty");
  await page.locator("html[data-visual-ready]").waitFor();

  const composer = page.locator('[data-slot="composer-root"]');
  const footer = page.locator('[data-slot="composer-footer"]');
  await expect(composer).toHaveCSS("border-top-width", "1px");
  await expect(footer).toHaveCSS("padding-bottom", "6px");
  await expect(footer).toHaveCSS("padding-right", "8px");
  await expect(page.getByRole("button", { name: "Attach image" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Switch model" })).toBeVisible();

  // Read from the token rather than restated: the reading column's width is a
  // design decision that moves, and a literal here asserts one revision of it
  // against every later one. What must hold is that the composer spans the column.
  const box = await composer.boundingBox();
  expect(box?.width).toBe(
    await page.evaluate(() =>
      Number.parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue("--content-max"),
      ),
    ),
  );
  // ONE edge mechanism, and for the composer that is the border above. The
  // tool-window style gives flush chrome no cast at all, so a depth shadow here
  // would be the second edge this test exists to forbid.
  expect(await composer.evaluate((element) => getComputedStyle(element).boxShadow)).toBe("none");

  const borderBeforeFocus = await composer.evaluate(
    (element) => getComputedStyle(element).borderTopColor,
  );
  await page.getByRole("textbox", { name: "Message composer" }).focus();
  await expect
    .poll(() => composer.evaluate((element) => getComputedStyle(element).borderTopColor))
    .not.toBe(borderBeforeFocus);
});

test("recovery action dismisses the problem and resends the last user input", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=recovery");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Retry" }).click();

  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-sent-input",
    /Review the Runtime boundary/,
  );
});

test("long content remains inside the reading column without horizontal overflow", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const stream = page.locator(".msg-scroll > .panel-scroll");
  const overflow = await stream.evaluate((element) => element.scrollWidth - element.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await expect(page.locator('[data-slot="composer-root"]')).toBeVisible();
});

// The composer floats over the transcript, so the transcript has to end above it.
// Nothing else can catch this: the tail is only reachable at full scroll, the
// overlap looks plausible on a fixture that fits its viewport, and the reservation
// is published by a ResizeObserver rather than written in a class — so it can be
// silently zero and every other assertion still passes.
for (const state of ["long-content", "question", "delegated"] as const) {
  test(`the floating composer reserves its own height at the tail of ${state}`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const clearance = await page.evaluate(async () => {
      const scroller = document.querySelector(".msg-scroll-viewport");
      const composer = document.querySelector('[data-slot="composer-root"]');
      if (!scroller || !composer) return null;
      scroller.scrollTop = scroller.scrollHeight;
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const tail = scroller.firstElementChild?.lastElementChild;
      if (!tail) return null;
      return Math.round(composer.getBoundingClientRect().top - tail.getBoundingClientRect().bottom);
    });

    // A number, not just "positive": the fade band above the input is scenery, and
    // a tail that ends inside it is a tail the user reads through a gradient.
    expect(clearance).toBeGreaterThanOrEqual(48);
  });
}

for (const theme of ["light", "dark"] as const) {
  for (const state of VISUAL_AGENT_STATES) {
    test(`agent golden ${theme} ${state}`, async ({ page }) => {
      await page.goto(`/visual/?fixture=agent&theme=${theme}&state=${state}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (state === "long-content") {
        await page.locator(".shiki-block .shiki").waitFor();
      }
      // Put the transcript where it belongs BEFORE the clock stops, rather than
      // waiting to see where it lands. Ready only means the tree is mounted;
      // use-stick-to-bottom then eases the scroll with Date.now(), and the
      // resting position it eases toward moves under it — `content-visibility`
      // gives off-screen blocks an estimated height until they are laid out, so
      // the same transcript settles a pixel apart between two runs and every
      // row in the frame shifts with it. Every fixture sticks to the bottom, and
      // the bottom is a hard stop the browser clamps to: assert it.
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

      // The fixture keeps Date.now advancing through production bootstrap so
      // use-stick-to-bottom can complete its frame waits. Freeze only after the
      // ready boundary: running reasoning retains its real elapsed-time label,
      // while consecutive screenshots observe the same instant.
      await page.evaluate(() => {
        const snapshotNow = Date.now();
        Date.now = () => snapshotNow;
      });

      await expect(page).toHaveScreenshot(`agent-${theme}-${state}.png`);
    });
  }
}
