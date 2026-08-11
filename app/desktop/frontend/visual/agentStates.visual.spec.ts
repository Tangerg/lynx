import { expect, test } from "@playwright/test";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";

const EXPECTED_ATTENTION: Record<VisualAgentState, string> = {
  empty: "idle",
  idle: "finished",
  running: "running",
  "answer-opening": "running",
  steer: "running",
  waiting: "waiting",
  question: "waiting",
  terminal: "finished",
  canceled: "finished",
  error: "finished",
  recovery: "finished",
  delegated: "running",
  "long-content": "finished",
  narrative: "finished",
  "tool-shells": "finished",
  waves: "running",
};

// The Record's own exhaustiveness is not enforced by its type — a partial Record
// still typechecks against an index signature — and an absent expectation reads to
// Playwright as "assert the attribute exists", which passes for every value.
// `narrative` had been missing since it was added. A state without an expectation
// is a state nobody is asserting anything about.
test("every declared state carries an expected attention", () => {
  expect(Object.keys(EXPECTED_ATTENTION).sort()).toEqual([...VISUAL_AGENT_STATES].sort());
});

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

// The edge is the only thing at the container level that says "this stopped the
// run to ask you something", and it is reached through a variant that spent its
// whole life rendering nothing — the two entries in the map were byte-identical, so
// a caller passing `warning` got no cue at all. Assert the tone reaches the box,
// and that it leaves with the question.
test("a pending approval carries a warning edge and a settled one carries none", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  const shell = page.locator('[data-slot="hitl-shell"]');
  await expect(shell).toHaveCSS("border-top-width", "1px");
  // The tone, not merely a line: this box used to be edgeless, and a neutral
  // hairline here would pass a width-only check while saying nothing.
  const edge = await shell.evaluate((element) => getComputedStyle(element).borderTopColor);
  const warning = await page.evaluate(() => {
    const probe = document.createElement("div");
    probe.style.color = "var(--color-warning-edge)";
    document.body.append(probe);
    const value = getComputedStyle(probe).color;
    probe.remove();
    return value;
  });
  expect(edge).toBe(warning);

  await page.getByRole("button", { name: /Approve/ }).click();
  await expect(shell).toHaveCount(0);
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
  await expect(page.getByRole("button", { name: "Stop" })).toBeVisible();

  await page.getByRole("button", { name: "Stop" }).click();
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

  // The banner's steps come from the session's plan snapshot now, not from a
  // per-run plan Item — same three steps, read from where the protocol keeps them.
  await expect(page.getByText("Run quality gates", { exact: true })).toBeVisible();
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

  // ONE edge mechanism, and for a panel resting ON the transcript that is a ring,
  // not a border: a drawn line was the only outlined object left on a screen whose
  // regions all separate by cast. So no border AND no second stroke — the ring and
  // the depth under it are the single `box-shadow` this asserts.
  await expect(composer).toHaveCSS("border-top-width", "0px");
  const material = await composer.evaluate((element) => {
    const probe = document.createElement("div");
    probe.style.boxShadow =
      "0 0 0 var(--composer-edge-width) color-mix(in oklab, var(--color-text) 14%, transparent), var(--shadow-composer-depth)";
    probe.style.background = "var(--app-composer-surface)";
    probe.style.backdropFilter = "var(--composer-backdrop)";
    document.body.append(probe);
    const expected = {
      shadow: getComputedStyle(probe).boxShadow,
      fill: getComputedStyle(probe).backgroundColor,
      backdrop: getComputedStyle(probe).backdropFilter,
    };
    probe.remove();
    const actual = getComputedStyle(element);
    return {
      expected,
      shadow: actual.boxShadow,
      fill: actual.backgroundColor,
      backdrop: actual.backdropFilter,
    };
  });
  expect(material.shadow).toBe(material.expected.shadow);
  // Translucent and blurred, or the ring reads as a stroke around a box rather than
  // as the edge of glass — the material is half of why the border could go.
  expect(material.fill).toBe(material.expected.fill);
  expect(material.fill).toMatch(/rgba|color\(|\/\s*0?\.\d/);
  expect(material.backdrop).toBe(material.expected.backdrop);
  expect(material.backdrop).not.toBe("none");

  const ringBeforeFocus = material.shadow;
  await page.getByRole("textbox", { name: "Message composer" }).focus();
  await expect
    .poll(() => composer.evaluate((element) => getComputedStyle(element).boxShadow))
    .not.toBe(ringBeforeFocus);
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

// The three seams around the reading plane are one primitive, and the top one is the
// easy one to lose: half a device pixel, so the raster comparison can pass on its
// absence, and the bars sit in their region's own colour with the body scrolling
// under them — with no seam the session title and the first line of a message share
// one field of white.
// Asserted on the MECHANISM, not on one bar: the edge used to hang off a bespoke
// class, which is why the page identity got one and the dock's tab strip — the bar
// right beside it, in the same visual row — got none.
test("every chrome bar that takes a bottom edge wears the style edge", async ({ page }) => {
  await page.goto("/visual/?fixture=workspace&theme=light&state=dock-light");
  await page.locator("html[data-visual-ready]").waitFor();

  const measured = await page.evaluate(() => {
    // Resolve the expected value THROUGH the engine rather than composing the two
    // token strings: computed `box-shadow` is normalised (`rgba(0, 0, 0, 0.2)`,
    // `0px`) and the tokens are not, so a string built here would only ever assert
    // that this test can reproduce Chromium's serialiser.
    const probe = document.createElement("div");
    probe.style.boxShadow = "var(--app-header-edge) var(--color-border)";
    document.body.append(probe);
    const edge = getComputedStyle(probe).boxShadow;
    probe.remove();
    const bars = [...document.querySelectorAll(".agent-surface-header")];
    return {
      edge,
      withEdge: bars
        .filter((bar) => bar.classList.contains("agent-surface-divider"))
        .map((bar) => getComputedStyle(bar).boxShadow),
      withoutEdge: bars
        .filter((bar) => !bar.classList.contains("agent-surface-divider"))
        .map((bar) => getComputedStyle(bar).boxShadow),
    };
  });

  expect(measured.withEdge.length).toBeGreaterThanOrEqual(2);
  for (const shadow of measured.withEdge) expect(shadow).toBe(measured.edge);
  // A bar that already butts against another region takes nothing.
  for (const shadow of measured.withoutEdge) expect(shadow).toBe("none");
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

    const measured = await page.evaluate(async () => {
      const scroller = document.querySelector(".msg-scroll-viewport");
      const composer = document.querySelector('[data-slot="composer-root"]');
      if (!scroller || !composer) return null;
      scroller.scrollTop = scroller.scrollHeight;
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const tail = scroller.firstElementChild?.lastElementChild;
      if (!tail) return null;
      return {
        clearance: Math.round(
          composer.getBoundingClientRect().top - tail.getBoundingClientRect().bottom,
        ),
        // The margin the contract adds on top of the panel's own height, read
        // rather than restated: `COMPOSER_CLEARANCE` spells it `+1rem`, and a
        // literal here would have to be kept in step with a class in another file.
        margin: Math.round(Number.parseFloat(getComputedStyle(document.documentElement).fontSize)),
      };
    });

    // Not merely positive: the panel is glass, so a tail resting against its top
    // edge is a tail the user reads through a blur.
    expect(measured?.margin).toBeGreaterThan(0);
    expect(measured!.clearance).toBeGreaterThanOrEqual(measured!.margin);
  });
}

// Every state collapses its tool calls into an "N steps" summary, so until this
// test the rows themselves — the app's most-read surface — appeared in no
// screenshot and in no browser assertion. What it pins is what a row REPORTS: the
// subject it acted on, and for an edit the lines it changed.
// The plan was on screen twice: the banner that stands above the transcript, and the
// tool row that wrote it. Nothing about that is visible to a golden — both readings look
// deliberate — so the assertion is that the transcript does not narrate a call whose
// surface already holds it.
test("a tool with a standing surface is not narrated as well", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  // The banner holds the plan.
  await expect(page.getByText("Review visual evidence").first()).toBeVisible();

  const stream = page.locator(".msg-scroll-viewport");
  // The transcript does not repeat it, closed or open.
  for (let i = 0; i < 6; i++) {
    const shut = stream.locator(
      "[data-slot='agent-activity-disclosure'] button[aria-expanded='false']",
    );
    if ((await shut.count()) === 0) break;
    await shut
      .first()
      .click({ timeout: 2000 })
      .catch(() => {});
  }
  // Its rendered label, not the tool name — the row shows "Update the plan".
  await expect(stream.getByText("Update the plan")).toHaveCount(0);

  // The calls it does narrate are still there — the filter removed one row, not the run.
  await expect(stream.getByText("atomicity_and_idempotency.go").first()).toBeVisible();
});

// The frame every turn passes through: the answer's item is open and still empty.
// Nothing may be folded here — an empty block is not an answer, and treating it as one
// collapsed the thinking to a one-line row with nothing in it, while the reply it
// deferred to had not written a character.
test("an opened but empty answer folds nothing behind it", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=answer-opening");
  await page.locator("html[data-visual-ready]").waitFor();

  const thinking = page
    .locator("[data-slot='agent-activity-disclosure']")
    .filter({ hasText: "Thinking" });
  await expect(thinking.locator("button[aria-expanded]").first()).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  // Its body, not just its summary row.
  await expect(thinking).toContainText("The framework must expose execution capability");

  // And the live work is still a list of steps rather than one folded wave.
  await expect(page.getByRole("button", { name: /steps/ })).toHaveCount(0);
});

test("expanded reasoning keeps a quiet identity mark and an aside rule", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waves");
  await page.locator("html[data-visual-ready]").waitFor();

  const reasoning = page
    .locator("[data-slot='agent-activity-disclosure']")
    .filter({ hasText: "Thinking" });
  const trigger = reasoning.locator("button[aria-expanded]").first();
  const mark = trigger.locator("span[aria-hidden]").first();

  await expect(reasoning).toHaveAttribute("data-shell", "line");
  await expect(mark.locator("svg")).toBeVisible();
  await expect(mark).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(reasoning.getByRole("region")).toHaveCSS("border-left-width", "1px");
});

test("an expanded tool row reports its subject and what it changed", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();

  const row = page
    .locator(".msg-scroll-viewport button")
    .filter({ hasText: "specialisedPreviewProjections.ts" })
    .first();
  await expect(row).toBeVisible();

  // The point of the split, in a real layout: the path is too long for the row, so
  // the DIRECTORY is the part that gets clipped and the filename is whole. Measured
  // rather than screenshotted because it is the overflow that matters, and a golden
  // cannot tell "clipped on the left" from "clipped on the right" without a human.
  const clipping = await row.evaluate((element) => {
    const directory = element.querySelector("[dir=rtl]");
    const filename = directory?.nextElementSibling?.nextElementSibling;
    return {
      directoryClipped: !!directory && directory.scrollWidth > directory.clientWidth + 1,
      directoryLost: directory ? directory.scrollWidth - directory.clientWidth : 0,
      filenameLost: filename ? filename.scrollWidth - filename.clientWidth : 0,
      filenameText: filename?.textContent,
    };
  });
  expect(clipping.directoryClipped).toBe(true);
  // The directory gives way FIRST and gives way further — that is the ordering this
  // pins, and it is the whole point of the atom. "The filename is never touched" is a
  // stronger claim than the layout can keep: a name wider than the column it is read in
  // has to ellipsize, because the alternative is pushing the row past its container,
  // which is what it used to do. It is still whole in the DOM, and in the title.
  expect(clipping.directoryLost).toBeGreaterThan(clipping.filenameLost);
  expect(clipping.filenameText).toBe("specialisedPreviewProjections.ts");
  // `+2 −1` from the call's own diff rows, drawn by the atom the diff views use —
  // one fact, one rendering, wherever it appears.
  await expect(row).toContainText("+2");
  await expect(row).toContainText("−1");
});

test("an expanded wave keeps its summary while its rows scroll past", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();

  const header = page.locator("[data-slot=agent-activity-disclosure] .sticky").first();
  await expect(header).toBeVisible();

  // Measured, not screenshotted: a golden of a scrolled transcript cannot tell
  // "the header stuck" from "the header happened to be in frame". The card's own
  // `overflow` decides this — `hidden` would make the card the scrollport and the
  // header would leave with its rows.
  const stuck = await header.evaluate((element) => {
    const viewport = element.closest(".msg-scroll-viewport");
    const card = element.parentElement;
    if (!viewport || !card) return null;
    const before = element.getBoundingClientRect().top - card.getBoundingClientRect().top;
    viewport.scrollTop = viewport.scrollHeight;
    return {
      before,
      overflow: getComputedStyle(card).overflow,
      position: getComputedStyle(element).position,
    };
  });
  expect(stuck?.position).toBe("sticky");
  // `hidden` here is the bug this guards: it silently turns the card into the
  // scrollport, and sticky then has nothing to stick to.
  expect(stuck?.overflow).toBe("clip");
});

test("the goal banner reports the allowance that will run out first", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const trigger = page.getByRole("button", { name: "Show the allowance" });
  await expect(trigger).toBeVisible();

  // Cost is at 90% and runs at 35%; steps carries the biggest number and no cap
  // at all. The collapsed row must name the axis that will stop the loop, not
  // the loudest number on it.
  await expect(trigger).toContainText("$4.50/$5.00");
  await expect(trigger).not.toContainText("7/20");

  await trigger.click();

  // Every axis once open — and the uncapped one SAYS so instead of drawing a
  // bar. A full track under "no limit" reads as "nearly spent", which is the
  // opposite of what it means and which a golden cannot tell from a bar that is
  // genuinely full.
  const banner = page
    .locator("[data-slot=agent-activity-disclosure]")
    .filter({ hasText: "green on Linux" });
  await expect(banner.locator("[role=progressbar]")).toHaveCount(2);
  await expect(banner.getByText("no limit")).toBeVisible();
});

for (const theme of ["light", "dark"] as const) {
  for (const state of VISUAL_AGENT_STATES) {
    test(`agent golden ${theme} ${state}`, async ({ page }) => {
      await page.goto(`/visual/?fixture=agent&theme=${theme}&state=${state}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (state === "long-content") {
        await page.locator(".shiki-block .shiki").waitFor();
      }
      // The canonical tool-shell frame exists to photograph the tool grammar,
      // so open its completed wave before capturing it. A collapsed "6 steps"
      // row cannot catch icon, status, grouping or preview regressions in the
      // components the state is named for.
      if (state === "tool-shells") {
        await page.getByRole("button", { name: /steps/ }).first().click();
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
