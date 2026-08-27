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

  await page.getByRole("button", { name: /Allow once/ }).click();

  await expect(page.getByText("Approved", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
});

test("a pending approval uses the Codex neutral request surface", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  const surface = page.locator('[data-slot="approval-surface"]');
  await expect(surface).toHaveCSS("border-top-width", "0px");
  await expect(surface).toHaveCSS("border-radius", "24px");
  await expect(surface.getByText("Terminal", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Run the race detector across the workspace before committing.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByText("Approval required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Medium risk", { exact: true })).toHaveCount(0);
  await expect(page.getByText("go test -race ./...", { exact: true })).toHaveCount(1);
  await expect(page.getByText("Run the race detector", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Approval options" })).toBeVisible();

  await page.getByRole("button", { name: /Allow once/ }).click();
  await expect(surface).toHaveCount(0);
});

test("HITL rejection preserves the same exact interrupt identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=waiting");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: /Deny/ }).click();

  await expect(page.getByText("Declined", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
});

test("question settlement uses the exact interrupt identity", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  const request = page.locator('[data-slot="question-request-surface"]');
  await expect(request).toBeVisible();
  await expect(request).toHaveCSS("border-radius", "24px");
  await expect(request).toHaveCSS("border-top-width", "0px");
  await expect(page.locator('[data-slot="composer-root"]')).toHaveCount(0);
  await expect(page.getByText("Input needed", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Gate", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("radio", { name: /Race detector/ })).toHaveAttribute(
    "aria-checked",
    "true",
  );

  await page.getByRole("radio", { name: /Race detector/ }).click();
  await page
    .getByRole("textbox", { name: "What should this gate protect?" })
    .fill("Runtime boundaries and cancellation paths.");
  await page.getByRole("button", { name: "Next", exact: true }).click();

  const settled = page.getByRole("button", { name: "Asked 2 questions" });
  await expect(settled).toBeVisible();
  await expect(settled).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByText("What should this gate protect?", { exact: true })).toHaveCount(0);
  await expect(settled.locator("xpath=../..")).toHaveScreenshot(
    "question-settled-collapsed-light.png",
  );
  await settled.click();
  await expect(page.getByText("What should this gate protect?", { exact: true })).toBeVisible();
  await expect(page.getByText("Runtime boundaries and cancellation paths.")).toBeVisible();
  await expect(settled.locator("xpath=../..")).toHaveScreenshot(
    "question-settled-expanded-light.png",
  );
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_question");
  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-resumed-response",
    JSON.stringify({
      type: "answer",
      answers: [["Race detector"], ["Runtime boundaries and cancellation paths."]],
    }),
  );
});

test("question skip sends real ordered empty answers and restores the composer", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Skip", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "What should this gate protect?" })).toBeVisible();
  await page.getByRole("button", { name: "Skip", exact: true }).click();

  await expect(page.locator("html")).toHaveAttribute(
    "data-visual-resumed-response",
    JSON.stringify({ type: "answer", answers: [[], []] }),
  );
  await expect(page.locator('[data-slot="question-request-surface"]')).toHaveCount(0);
  await expect(page.locator('[data-slot="composer-root"]')).toBeVisible();
});

test("question choices keep their descriptions inline without a comparison sidecar", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("radio", { name: /Race detector/ })).toContainText(
    "Exercise concurrency and cancellation paths.",
  );
  await expect(page.getByRole("region", { name: "Race detector" })).toHaveCount(0);
  await expect(page.getByText("go test -race ./...")).toHaveCount(0);
  await expect(page.getByText("npm run test:visual")).toHaveCount(0);
  await expect(page).toHaveScreenshot("agent-light-question-preview.png");
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

test("a running Goal exposes Pause while the active turn exposes Stop", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  await expect(page.getByRole("button", { name: "Clear goal", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Pause goal", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Edit goal", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toHaveCount(1);
});

for (const theme of ["light", "dark"] as const) {
  test(`Goal is a compact composer mode without duplicate fields ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=idle`);
    await page.locator("html[data-visual-ready]").waitFor();

    const composerFooter = page.locator('[data-slot="composer-footer"]');
    const input = page.getByRole("textbox", { name: "Message composer" });
    await input.fill("/goal");
    await input.press("Enter");

    const mode = page.getByRole("button", { name: "Exit Goal mode" });
    await expect(mode).toBeVisible();
    await expect(mode).toHaveAttribute("aria-pressed", "true");
    await expect(input).toHaveValue("");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("spinbutton")).toHaveCount(0);
    await expect(composerFooter).toHaveScreenshot(`goal-composer-mode-${theme}.png`);

    await mode.click();
    await expect(mode).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`the standing Goal opens the compact objective editor ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=running`);
    await page.locator("html[data-visual-ready]").waitFor();

    await page.getByRole("button", { name: "Edit goal", exact: true }).click();

    const dialog = page.getByRole("dialog", { name: "Edit goal" });
    const backdrop = page.locator('[data-slot="text-editor-backdrop"]');
    const objective = dialog.getByRole("textbox", { name: "Goal" });
    await expect(dialog).toBeVisible();
    await expect(backdrop).toHaveCSS("background-color", "rgba(0, 0, 0, 0.133)");
    await expect(objective).toHaveValue(
      "Get the desktop suite green on Linux without loosening any gate or skipping a test",
    );
    await expect(objective).toHaveAttribute("rows", "12");
    await expect(dialog.getByRole("button", { name: "Save" })).toBeDisabled();
    await expect(dialog).toHaveScreenshot(`goal-editor-${theme}.png`);
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).not.toBeVisible();
  });
}

test("the compact Plan pill reveals the production checklist on hover", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  await page.getByRole("button", { name: "Step 2 / 3" }).hover();

  // The tooltip's steps come from the session's plan snapshot, not from a
  // per-run plan Item — same three steps, read from where the protocol keeps them.
  await expect(page.getByText("Run quality gates", { exact: true })).toBeVisible();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toHaveScreenshot("active-plan-tooltip-light.png");
});

test("the active plan stays with the composer instead of claiming the transcript header", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const plan = page.getByRole("button", { name: "Step 2 / 3" });
  const goal = page.locator('[data-slot="goal-status-row"]');
  const planBox = await plan.boundingBox();
  const goalBox = await goal.boundingBox();

  expect(planBox).not.toBeNull();
  expect(goalBox).not.toBeNull();
  expect(goalBox!.y - (planBox!.y + planBox!.height)).toBeGreaterThanOrEqual(0);
  expect(goalBox!.y - (planBox!.y + planBox!.height)).toBeLessThanOrEqual(16);
});

test("the standing goal stays in the composer stack instead of claiming the transcript header", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const goal = page.locator('[data-slot="composer-top-tray-surface"]');
  const composer = page.locator('[data-slot="composer-root"]');
  const goalBox = await goal.boundingBox();
  const composerBox = await composer.boundingBox();

  expect(goalBox).not.toBeNull();
  expect(composerBox).not.toBeNull();
  expect(Math.abs(composerBox!.x - goalBox!.x)).toBeLessThanOrEqual(1);
  expect(Math.abs(composerBox!.width - goalBox!.width)).toBeLessThanOrEqual(1);
  expect(composerBox!.y - (goalBox!.y + goalBox!.height)).toBeGreaterThanOrEqual(-1);
  expect(composerBox!.y - (goalBox!.y + goalBox!.height)).toBeLessThanOrEqual(0);
  await expect(goal.locator('[data-slot="goal-glyph"]')).toBeVisible();
});

test("the composer context ring exposes the Runtime window occupancy", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const gauge = page.getByRole("img", { name: "Context usage: 38%" });
  await expect(gauge).toBeVisible();
  await gauge.hover();

  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toContainText("Context window:");
  await expect(tooltip).toContainText("38% used (62% left)");
  await expect(tooltip).toContainText("96k / 256k tokens used");
});

for (const theme of ["light", "dark"] as const) {
  test(`a ${theme} user turn uses the Codex-neutral bubble material and geometry`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=idle`);
    await page.locator("html[data-visual-ready]").waitFor();

    // Codex gives the human turn a stable semantic hook and a neutral ink wash:
    // the bubble distinguishes ownership without turning every prompt into an
    // accent/status callout. Pin both schemes because a light-only assertion can
    // accidentally accept a translucent accent whose dark result is much louder.
    const bubble = page.locator("[data-user-message-bubble]");
    await expect(bubble).toHaveCount(1);
    await expect(bubble).toContainText("Review the Runtime boundary");

    const material = await bubble.evaluate((element) => {
      const probe = document.createElement("div");
      probe.style.background = "color-mix(in srgb, var(--color-text) 5%, transparent)";
      document.body.append(probe);
      const expectedBackground = getComputedStyle(probe).backgroundColor;
      probe.remove();

      const actual = getComputedStyle(element);
      return {
        background: actual.backgroundColor,
        expectedBackground,
        maxWidth: actual.maxWidth,
        padding: [actual.paddingTop, actual.paddingRight, actual.paddingBottom, actual.paddingLeft],
        radius: actual.borderRadius,
      };
    });

    expect(material).toEqual({
      background: material.expectedBackground,
      expectedBackground: material.expectedBackground,
      maxWidth: "77%",
      padding: ["8px", "12px", "8px", "12px"],
      radius: "16px",
    });
  });
}

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

test("the projectless composer owns Codex's inset rear project tray", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=empty");
  await page.locator("html[data-visual-ready]").waitFor();

  const tray = page.locator('[data-slot="composer-top-tray-surface"]');
  const composer = page.locator('[data-slot="composer-root"]');
  const footer = page.locator('[data-slot="composer-footer"]');
  const trayBox = await tray.boundingBox();
  const composerBox = await composer.boundingBox();

  expect(trayBox).not.toBeNull();
  expect(composerBox).not.toBeNull();
  expect(trayBox!.x - composerBox!.x).toBe(12);
  expect(composerBox!.x + composerBox!.width - (trayBox!.x + trayBox!.width)).toBe(12);
  expect(composerBox!.y - trayBox!.y).toBe(37);
  expect(trayBox!.y + trayBox!.height - composerBox!.y).toBe(22);
  await expect(tray.getByRole("button", { name: "Choose project" })).toBeVisible();
  await expect(tray.locator("svg")).toHaveCount(1);
  await expect(footer.getByRole("button", { name: "Choose project" })).toHaveCount(0);
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

test("code blocks stay readable and expose the wrap control", async ({ context, page }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: "http://127.0.0.1:4174",
  });
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const code = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
  await expect(code).toContainText("Execute(context.Context");
  const wrapControls = page.getByRole("button", { name: "Enable word wrap" });
  await expect(wrapControls).toHaveCount(3);
  await expect(wrapControls.first()).toHaveAttribute("aria-pressed", "false");
  await wrapControls.first().click();

  const wrappedControls = page.getByRole("button", { name: "Disable word wrap" });
  await expect(wrappedControls).toHaveCount(3);
  await expect(wrappedControls.first()).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator('.shiki-body[data-wrap="true"]')).toHaveCount(3);
  await expect(page.locator("iframe")).toHaveCount(0);
  await expect(page.locator(".shiki-block").filter({ hasText: "parent.postMessage" })).toHaveCount(
    1,
  );

  await code.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const selection = getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
  await page.keyboard.press("ControlOrMeta+C");
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(
      [
        "type Executor interface {",
        "    Execute(context.Context, Request) (Result, error)",
        "}",
      ].join("\n"),
    );

  const svgPreview = page.getByRole("img", { name: "Image generated by the assistant" });
  await expect(svgPreview).toBeVisible();
  const svgArtifact = page.locator(".shiki-block").filter({ has: svgPreview });
  const svgCopy = svgArtifact.getByRole("button", { name: "Copy code" });
  // `toBeVisible` may scroll the artifact underneath the pointer left by the
  // earlier wrap click. Move it away so this measures the true resting state.
  await page.mouse.move(0, 0);
  await expect.poll(() => svgCopy.evaluate((button) => getComputedStyle(button).opacity)).toBe("0");
  await svgArtifact.hover();
  await expect.poll(() => svgCopy.evaluate((button) => getComputedStyle(button).opacity)).toBe("1");
  await expect(svgArtifact.locator(".shiki-preview-body")).toHaveAttribute("tabindex", "0");
  await expect
    .poll(() => svgPreview.evaluate((image: HTMLImageElement) => image.naturalWidth))
    .toBe(240);
});

test("code blocks use the Codex caption and source geometry", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const block = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
  const geometry = await block.evaluate((root) => {
    const header = root.querySelector<HTMLElement>('[data-markdown-copy="exclude"]');
    const language = Array.from(header?.querySelectorAll("span") ?? []).find(
      (element) => element.textContent?.trim() === "go",
    );
    const source = root.querySelector<HTMLElement>(".shiki");
    if (!header || !language || !source) return null;
    const headerStyle = getComputedStyle(header);
    const languageStyle = getComputedStyle(language);
    const sourceStyle = getComputedStyle(source);
    const blockStyle = getComputedStyle(root);
    return {
      headerBackground: headerStyle.backgroundColor,
      blockMargin: blockStyle.marginBlockStart,
      headerPadding: `${headerStyle.paddingBlockStart} ${headerStyle.paddingInlineStart}`,
      bodyFamily: getComputedStyle(document.body).fontFamily,
      languageFamily: languageStyle.fontFamily,
      languageSize: languageStyle.fontSize,
      languageTransform: languageStyle.textTransform,
      sourcePadding: sourceStyle.paddingInlineStart,
      sourceMaxHeight: sourceStyle.maxHeight,
    };
  });

  expect(geometry).not.toBeNull();
  expect.soft(geometry?.headerBackground).toBe("rgba(0, 0, 0, 0)");
  expect.soft(geometry?.blockMargin).toBe("14px");
  expect.soft(geometry?.headerPadding).toBe("4px 8px");
  expect.soft(geometry?.languageFamily).toBe(geometry?.bodyFamily);
  expect.soft(geometry?.languageSize).toBe("14px");
  expect.soft(geometry?.languageTransform).toBe("none");
  expect.soft(geometry?.sourcePadding).toBe("8px");
  expect.soft(geometry?.sourceMaxHeight).toBe("none");
});

for (const theme of ["light", "dark"] as const) {
  test(`code block keeps its Codex material ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();
    const block = page.locator(".shiki-block").filter({ hasText: "Execute(context.Context" });
    await expect(block).toHaveScreenshot(`markdown-code-block-${theme}.png`);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`Mermaid is a semantic, copyable, zoomable artifact ${theme}`, async ({ context, page }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: "http://127.0.0.1:4174",
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const diagram = page.getByRole("img", { name: "Diagram" });
    await expect(diagram).toBeVisible();
    const artifact = diagram.locator("..");
    await artifact.hover();
    await expect(artifact).toHaveScreenshot(`markdown-mermaid-${theme}.png`);

    await artifact.getByRole("button", { name: "Copy Mermaid" }).click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toContain("```mermaid\ngraph LR");

    await artifact.evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      const selection = getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
    });
    await page.keyboard.press("ControlOrMeta+C");
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe("```mermaid\ngraph LR\n  Runtime --> Desktop\n  Desktop --> Frontend\n```");

    await artifact.getByRole("button", { name: "Enlarge diagram" }).click();
    await expect(page.getByRole("dialog", { name: "Diagram" })).toBeVisible();
    await page.keyboard.press("Escape");
  });

  test(`Markdown tables open a Codex reading preview ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const table = page.locator("[data-markdown-table]").filter({ hasText: "Boundary" });
    await table.evaluate((element) => element.scrollIntoView({ block: "center" }));
    await table.hover();
    await page.getByRole("button", { name: "Expand table" }).click();

    const dialog = page.getByRole("dialog", { name: "Table preview" });
    await expect(dialog).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await expect(dialog).toHaveCSS("scale", "none");
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(dialog).toHaveScreenshot(`markdown-table-preview-${theme}.png`);
    await page.getByRole("button", { name: "Close table preview" }).click();
    await expect(dialog).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`tables keep semantic alignment and copy their Markdown source ${theme}`, async ({
    context,
    page,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: "http://127.0.0.1:4174",
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const table = page.locator("[data-markdown-table]").filter({ hasText: "Run lifecycle" });
    await expect(table.locator("table")).toHaveAttribute("dir", "auto");
    await expect(table.locator("td.md-table-cell-numeric")).toHaveCount(2);

    await table.hover();
    await expect(table).toHaveScreenshot(`markdown-table-${theme}.png`);
    await table.getByRole("button", { name: "Copy table" }).click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toContain("| Boundary | Owner | Checks |");
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`Markdown media previews inline data without requesting remote URLs ${theme}`, async ({
    page,
  }) => {
    let remoteRequests = 0;
    await page.route("https://tracker.example/**", async (route) => {
      remoteRequests += 1;
      await route.abort();
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const blocked = page.getByRole("button", { name: "Tracking pixel" });
    await expect(blocked).toBeDisabled();
    await expect(page.locator('img[src^="https://tracker.example/"]')).toHaveCount(0);
    expect(remoteRequests).toBe(0);

    const preview = page.getByRole("button", { name: "Inline architecture" });
    await expect(page.locator('[data-markdown-image-grid="true"] > button')).toHaveCount(2);
    await expect(preview.locator("img")).toHaveAttribute("loading", "lazy");
    await preview.evaluate((button) => button.parentElement?.scrollIntoView({ block: "center" }));
    await expect(preview).toBeVisible();
    await expect(preview).toHaveScreenshot(`markdown-image-${theme}.png`);
    await preview.click();
    const dialog = page.getByRole("dialog", { name: "Inline architecture" });
    await expect(dialog).toBeVisible();
    // The dialog is fixed but its 90% backdrop deliberately preserves the
    // transcript behind it. Pin that background before the golden: the
    // transcript's follow animation and `scrollIntoView` otherwise race over
    // which equally valid 49px slice shows through the backdrop.
    const transcript = page.locator(".msg-scroll-viewport");
    await transcript.evaluate((viewport) => {
      viewport.scrollTop = 0;
    });
    await expect.poll(() => transcript.evaluate((viewport) => viewport.scrollTop)).toBe(0);
    const controlSizes = await Promise.all(
      ["Download image", "Close image preview", "Zoom out image", "Zoom in image"].map((name) =>
        page.getByRole("button", { name }).evaluate((button, accessibleName) => {
          const element = button as HTMLElement;
          return {
            accessibleName,
            width: element.offsetWidth,
            height: element.offsetHeight,
          };
        }, name),
      ),
    );
    for (const { accessibleName, width, height } of controlSizes) {
      expect.soft(width, `${accessibleName} width`).toBeGreaterThanOrEqual(40);
      expect.soft(height, `${accessibleName} height`).toBeGreaterThanOrEqual(40);
    }
    await expect(dialog).toHaveScreenshot(`markdown-image-lightbox-${theme}.png`);
    await page.getByRole("button", { name: "Zoom in image" }).click();
    await expect(dialog.locator('[data-image-zoom="125"]')).toBeVisible();
    await page.getByRole("button", { name: "Next image" }).click();
    await expect(page.getByRole("dialog", { name: "Inline detail" })).toBeVisible();
    await expect(page.locator('[data-image-zoom="100"]')).toBeVisible();
    await page.keyboard.press("ArrowLeft");
    await expect(page.getByRole("dialog", { name: "Inline architecture" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Inline architecture" })).toHaveCount(0);
  });
}

test("context compaction uses the Codex activity row without divider chrome", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=narrative");
  await page.locator("html[data-visual-ready]").waitFor();

  const compaction = page.getByRole("button", { name: "Context automatically compacted" });
  await compaction.scrollIntoViewIfNeeded();
  await expect(compaction.locator(".lucide-minimize-2")).toBeVisible();
  await expect(compaction.locator("xpath=..").locator(".h-px")).toHaveCount(0);
  await expect(compaction).toHaveAttribute("aria-expanded", "false");
  await expect(compaction.locator("xpath=..")).toHaveScreenshot("context-compaction-light.png");

  await compaction.click();
  await expect(compaction).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByText("Earlier tool output folded into a summary.")).toBeVisible();
});

test("Markdown structural primitives follow the Codex reading grammar", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();

  const markdown = page.locator(".md").filter({ hasText: "Structural primitives" });
  const styles = await markdown.evaluate((root) => {
    const h2 = Array.from(root.querySelectorAll("h2")).find((heading) =>
      heading.textContent?.includes("Architecture review"),
    );
    const h3 = Array.from(root.querySelectorAll("h3")).find((heading) =>
      heading.textContent?.includes("Structural primitives"),
    );
    const primaryList = Array.from(root.querySelectorAll("ul")).find((list) =>
      list.textContent?.includes("Primary marker"),
    );
    const leadParagraph = Array.from(root.querySelectorAll(":scope > p")).find((paragraph) =>
      paragraph.textContent?.includes("The consumer owns persistence policy"),
    );
    const leadList = leadParagraph?.nextElementSibling;
    const tableContainer = root.querySelector(".md-table-container");
    const table = tableContainer?.querySelector("table");
    const tableHeader = table?.querySelector("th");
    const proseParagraph = Array.from(root.querySelectorAll(":scope > p")).find((paragraph) =>
      paragraph.textContent?.includes("A deliberately long final paragraph"),
    );
    const inlineCode = proseParagraph?.querySelector("code");
    const nestedList = primaryList?.querySelector(":scope > li > ul");
    const deepList = nestedList?.querySelector(":scope > li > ul");
    const rtlList = Array.from(root.querySelectorAll("ul")).find((list) =>
      list.textContent?.includes("المرحلة الأولى"),
    );
    const taskList = root.querySelector("ol.contains-task-list");
    const looseTask = taskList?.querySelector("li.task-list-item:has(> p)");
    const looseTaskCheckbox = looseTask?.firstElementChild;
    const looseTaskParagraphs = looseTask?.querySelectorAll(":scope > p");
    const quote = root.querySelector("blockquote");
    const rule = root.querySelector("hr");
    if (
      !h2 ||
      !h3 ||
      !leadParagraph ||
      !(leadList instanceof HTMLUListElement) ||
      !tableContainer ||
      !table ||
      !tableHeader ||
      !proseParagraph ||
      !inlineCode ||
      !nestedList ||
      !deepList ||
      !rtlList ||
      !taskList ||
      !looseTask ||
      !(looseTaskCheckbox instanceof HTMLInputElement) ||
      looseTaskParagraphs?.length !== 2 ||
      !quote ||
      !rule
    )
      return null;
    const h2Style = getComputedStyle(h2);
    const h3Style = getComputedStyle(h3);
    const leadParagraphStyle = getComputedStyle(leadParagraph);
    const leadListStyle = getComputedStyle(leadList);
    const tableContainerStyle = getComputedStyle(tableContainer);
    const tableStyle = getComputedStyle(table);
    const tableHeaderStyle = getComputedStyle(tableHeader);
    const proseParagraphStyle = getComputedStyle(proseParagraph);
    const inlineCodeStyle = getComputedStyle(inlineCode);
    const rtlListStyle = getComputedStyle(rtlList);
    return {
      h2Size: h2Style.fontSize,
      h2Margin: `${h2Style.marginBlockStart} ${h2Style.marginBlockEnd}`,
      h3Size: h3Style.fontSize,
      h3Margin: `${h3Style.marginBlockStart} ${h3Style.marginBlockEnd}`,
      leadParagraphMargin: `${leadParagraphStyle.marginBlockStart} ${leadParagraphStyle.marginBlockEnd}`,
      leadListMargin: `${leadListStyle.marginBlockStart} ${leadListStyle.marginBlockEnd}`,
      tableMargin: `${tableContainerStyle.marginBlockStart} ${tableContainerStyle.marginBlockEnd}`,
      tableFontSize: tableStyle.fontSize,
      tableLineHeight: tableStyle.lineHeight,
      tableHeaderFontSize: tableHeaderStyle.fontSize,
      tableHeaderLineHeight: tableHeaderStyle.lineHeight,
      proseParagraphMargin: `${proseParagraphStyle.marginBlockStart} ${proseParagraphStyle.marginBlockEnd}`,
      inlineCodeDecoration:
        inlineCodeStyle.getPropertyValue("box-decoration-break") ||
        inlineCodeStyle.getPropertyValue("-webkit-box-decoration-break"),
      inlineCodeFontSize: inlineCodeStyle.fontSize,
      inlineCodeRadius: inlineCodeStyle.borderRadius,
      inlineCodeWordBreak: inlineCodeStyle.wordBreak,
      inlineCodeWrap: inlineCodeStyle.overflowWrap,
      rtlDirection: rtlListStyle.direction,
      rtlStartPadding: rtlListStyle.paddingInlineStart,
      rtlEndPadding: rtlListStyle.paddingInlineEnd,
      nestedMarker: getComputedStyle(nestedList).listStyleType,
      deepMarker: getComputedStyle(deepList).listStyleType,
      taskMarker: getComputedStyle(taskList).listStyleType,
      looseTaskDisplay: getComputedStyle(looseTask).display,
      looseTaskColumns: getComputedStyle(looseTask).gridTemplateColumns,
      looseTaskCheckboxInset: getComputedStyle(looseTaskCheckbox).marginTop,
      looseTaskFollowUpColumn: getComputedStyle(looseTaskParagraphs[1]!).gridColumnStart,
      quoteInset: getComputedStyle(quote).paddingInlineStart,
      quoteRule: getComputedStyle(quote, "::after").width,
      ruleMargin: getComputedStyle(rule).marginBlockStart,
    };
  });

  expect(styles).not.toBeNull();
  expect.soft(styles?.h2Size).toBe("20px");
  expect.soft(styles?.h2Margin).toBe("20px 10px");
  expect.soft(styles?.h3Size).toBe("17px");
  expect.soft(styles?.h3Margin).toBe("20px 10px");
  expect.soft(styles?.leadParagraphMargin).toBe("0px 10px");
  expect.soft(styles?.leadListMargin).toBe("0px 10px");
  expect.soft(styles?.tableMargin).toBe("0px 0px");
  expect.soft(styles?.tableFontSize).toBe("14px");
  expect.soft(styles?.tableLineHeight).toBe("21px");
  expect.soft(styles?.tableHeaderFontSize).toBe("14px");
  expect.soft(styles?.tableHeaderLineHeight).toBe("16px");
  expect.soft(styles?.proseParagraphMargin).toBe("0px 11px");
  expect.soft(styles?.inlineCodeDecoration).toBe("clone");
  expect.soft(styles?.inlineCodeFontSize).toBe("14.72px");
  expect.soft(styles?.inlineCodeRadius).toBe("6px");
  expect.soft(styles?.inlineCodeWordBreak).toBe("break-word");
  expect.soft(styles?.inlineCodeWrap).toBe("anywhere");
  expect.soft(styles?.rtlDirection).toBe("rtl");
  expect.soft(styles?.rtlStartPadding).toBe("21px");
  expect.soft(styles?.rtlEndPadding).toBe("0px");
  expect.soft(styles?.nestedMarker).toBe("circle");
  expect.soft(styles?.deepMarker).toBe("square");
  expect.soft(styles?.taskMarker).toBe("none");
  expect.soft(styles?.looseTaskDisplay).toBe("grid");
  expect.soft(styles?.looseTaskColumns).not.toBe("none");
  expect.soft(styles?.looseTaskCheckboxInset).toBe("4px");
  expect.soft(styles?.looseTaskFollowUpColumn).toBe("2");
  expect.soft(styles?.quoteInset).toBe("24px");
  expect.soft(styles?.quoteRule).toBe("4px");
  expect.soft(styles?.ruleMargin).toBe("28px");
});

for (const theme of ["light", "dark"] as const) {
  test(`wrapped inline code keeps the Codex cloned well in ${theme}`, async ({ page }) => {
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`);
    await page.locator("html[data-visual-ready]").waitFor();

    const paragraph = page
      .locator(".md > p")
      .filter({ hasText: "A deliberately long final paragraph" });
    const inlineCode = paragraph.locator("code");
    await expect(inlineCode).toContainText("expectedRuntimeProjectionRevisionIdentifier");
    expect(await inlineCode.evaluate((element) => element.getClientRects().length)).toBeGreaterThan(
      1,
    );
    await expect(paragraph).toHaveScreenshot(`inline-code-wrap-${theme}.png`);
  });
}

// The three seams around the reading plane are one primitive, and the top one is the
// easy one to lose: half a device pixel, so the raster comparison can pass on its
// absence, and the bars sit in their region's own colour with the body scrolling
// under them — with no seam the session title and the first line of a message share
// one field of white.
// Assert the shared mechanism so every chrome bar in the same visual row receives the
// same seam contract.
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

// The input rung floats over the transcript, so the transcript has to end above it.
// Nothing else can catch this: the tail is only reachable at full scroll, the
// overlap looks plausible on a fixture that fits its viewport, and the reservation
// is published by a ResizeObserver rather than written in a class — so it can be
// silently zero and every other assertion still passes.
for (const { state, inputSurface } of [
  { state: "long-content", inputSurface: '[data-slot="composer-root"]' },
  { state: "question", inputSurface: '[data-slot="question-request-surface"]' },
  { state: "delegated", inputSurface: '[data-slot="composer-root"]' },
] as const) {
  test(`the floating input surface reserves its own height at the tail of ${state}`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const measured = await page.evaluate(async (inputSurface) => {
      const scroller = document.querySelector(".msg-scroll-viewport");
      const input = document.querySelector(inputSurface);
      if (!scroller || !input) return null;
      scroller.scrollTop = scroller.scrollHeight;
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const tail = scroller.firstElementChild?.lastElementChild;
      if (!tail) return null;
      return {
        clearance: Math.round(
          input.getBoundingClientRect().top - tail.getBoundingClientRect().bottom,
        ),
        // The margin the contract adds on top of the panel's own height, read
        // rather than restated: `COMPOSER_CLEARANCE` guarantees this `1rem`
        // after its scroll-rounding guard, and a literal here would have to be
        // kept in step with a class in another file.
        margin: Math.round(Number.parseFloat(getComputedStyle(document.documentElement).fontSize)),
      };
    }, inputSurface);

    // Not merely positive: a tail resting against the surface edge is visually
    // crowded and can remain behind the composer's translucent material.
    expect(measured?.margin).toBeGreaterThan(0);
    expect(measured!.clearance).toBeGreaterThanOrEqual(measured!.margin);
  });
}

for (const { state, action } of [{ state: "waiting", action: "Allow once" }] as const) {
  test(`compact ${state} opens with its blocking action above the composer`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 577 });
    await page.goto(`/visual/?fixture=agent&theme=light&state=${state}`);
    await page.locator("html[data-visual-ready]").waitFor();

    const composer = page.locator('[data-slot="composer-root"]');
    const button = page.getByRole("button", { name: action, exact: true });
    await expect(composer).toBeVisible();
    await expect(button).toBeVisible();
    await expect
      .poll(() =>
        page
          .locator(".msg-scroll-viewport")
          .evaluate((element) =>
            Number.parseFloat(getComputedStyle(element).getPropertyValue("--composer-overlay")),
          ),
      )
      .toBeGreaterThan(0);

    const clearance = await Promise.all([button.boundingBox(), composer.boundingBox()]);
    expect(clearance[0]!.y + clearance[0]!.height).toBeLessThanOrEqual(clearance[1]!.y);
  });
}

test("compact question replaces the composer with its blocking request", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 577 });
  await page.goto("/visual/?fixture=agent&theme=light&state=question");
  await page.locator("html[data-visual-ready]").waitFor();

  const request = page.locator('[data-slot="question-request-surface"]');
  const skip = page.getByRole("button", { name: "Skip", exact: true });
  await expect(request).toBeVisible();
  await expect(page.locator('[data-slot="composer-root"]')).toHaveCount(0);
  await expect(skip).toBeVisible();
  await expect
    .poll(() =>
      page
        .locator(".msg-scroll-viewport")
        .evaluate((element) =>
          Number.parseFloat(getComputedStyle(element).getPropertyValue("--composer-overlay")),
        ),
    )
    .toBeGreaterThan(0);

  const [requestBox, skipBox] = await Promise.all([request.boundingBox(), skip.boundingBox()]);
  expect(skipBox!.y).toBeGreaterThanOrEqual(requestBox!.y);
  expect(skipBox!.y + skipBox!.height).toBeLessThanOrEqual(requestBox!.y + requestBox!.height);
});

test("async transcript materialization follows only while the reader stays at the tail", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=long-content");
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
  await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();

  const measured = await page.evaluate(async () => {
    const scroller = document.querySelector<HTMLElement>(".msg-scroll-viewport");
    const content = scroller?.firstElementChild;
    if (!scroller || !content) return null;

    const grow = (height: number) => {
      const probe = document.createElement("div");
      probe.style.height = `${height}px`;
      probe.style.flex = `0 0 ${height}px`;
      content.append(probe);
      return probe;
    };
    const settle = () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      );

    scroller.scrollTop = scroller.scrollHeight;
    await settle();
    const followedProbe = grow(160);
    await settle();
    const followedDistance = scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop;

    scroller.dispatchEvent(new WheelEvent("wheel", { bubbles: true, deltaY: -220 }));
    scroller.scrollTop = Math.max(0, scroller.scrollTop - 220);
    await settle();
    const readerTop = scroller.scrollTop;
    const escapedDistance = scroller.scrollHeight - scroller.clientHeight - readerTop;

    const escapedProbe = grow(180);
    await settle();
    const afterGrowth = {
      top: scroller.scrollTop,
      distance: scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop,
    };

    followedProbe.remove();
    escapedProbe.remove();
    return { followedDistance, readerTop, escapedDistance, afterGrowth };
  });

  expect(measured).not.toBeNull();
  expect(measured!.followedDistance).toBeLessThanOrEqual(1);
  expect(measured!.afterGrowth.top).toBe(measured!.readerTop);
  expect(measured!.afterGrowth.distance - measured!.escapedDistance).toBe(180);
});

// Every state collapses its tool calls into an "N steps" summary, so until this
// test the rows themselves — the app's most-read surface — appeared in no
// screenshot and in no browser assertion. What it pins is what a row REPORTS: the
// subject it acted on, and for an edit the lines it changed.
// The plan was on screen twice: the active surface above the composer, and the
// tool row that wrote it. Nothing about that is visible to a golden — both readings look
// deliberate — so the assertion is that the transcript does not narrate a call whose
// surface already holds it.
test("a tool with a standing surface is not narrated as well", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  // The composer-owned pill holds the plan in its Codex-style hover surface.
  await page.getByRole("button", { name: "Step 2 / 3" }).hover();
  await expect(page.getByText("Review visual evidence", { exact: true })).toBeVisible();

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

test("an expanded patch reports only its call-scoped file receipt", async ({ page }) => {
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
    // The visual fixture has a deliberate 1120px minimum canvas. Constrain this
    // production row itself to exercise the dock/composer-narrowing case without
    // replacing the app layout with a test-only viewport implementation.
    const activity = element.closest<HTMLElement>("[data-slot='agent-activity-disclosure']");
    if (activity) activity.style.width = "480px";
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
  // stronger claim than the layout can keep: a name wider than its column must
  // ellipsize rather than push the row past its container. It remains whole in the DOM
  // and in the title.
  expect(clipping.directoryLost).toBeGreaterThan(clipping.filenameLost);
  expect(clipping.filenameText).toBe("specialisedPreviewProjections.ts");
  await expect(row).not.toContainText("+");
  await expect(row).not.toContainText("−");

  await row.click();
  const receipt = page.locator('[data-patch-change="modified"]').filter({
    has: page.getByTitle(
      "/Users/visual/scope/app/desktop/frontend/src/plugins/builtin/chat/tools/application/specialisedPreviewProjections.ts",
    ),
  });
  await expect(receipt).toContainText("Edited");
});

test("tool invocations stay on the transparent Codex work-narrative plane", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();

  const rows = page.locator(
    "[data-slot='agent-activity-disclosure'][data-tool='shell'], " +
      "[data-slot='agent-activity-disclosure'][data-tool='apply_patch']",
  );
  expect(await rows.count()).toBeGreaterThanOrEqual(5);

  for (let index = 0; index < (await rows.count()); index += 1) {
    const row = rows.nth(index);
    await expect(row).toHaveAttribute("data-shell", "line");
    await expect(row).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(row).toHaveCSS("border-top-width", "0px");
  }

  await page.mouse.move(0, 0);
  const closedChevron = rows
    .filter({ has: page.locator("button[aria-expanded='false']") })
    .first()
    .locator('[data-slot="agent-activity-chevron"]');
  await expect(closedChevron).toHaveCSS("opacity", "0");
});

test("completed work folds before the separate final answer owns message actions", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();

  const assistantTurns = page.getByRole("heading", { name: "Assistant" });
  await expect(assistantTurns).toHaveCount(2);

  const work = assistantTurns.nth(0).locator("..");
  await expect(work.getByRole("button", { name: /6 steps/ })).toBeVisible();
  await expect(work.getByRole("button", { name: "Copy message" })).toHaveCount(0);

  const answer = assistantTurns.nth(1).locator("..");
  await expect(answer).toContainText("The boundary is clean");
  await expect(answer.getByRole("button", { name: "Copy message" })).toBeVisible();
  await expect(answer.getByRole("button", { name: "Regenerate response" })).toBeVisible();
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

test("the Goal surface stays quiet and omits Runtime constraints", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=running");
  await page.locator("html[data-visual-ready]").waitFor();

  const row = page.locator('[data-slot="goal-status-row"]');
  await expect(row).toContainText("Pursuing goal");
  await expect(row).toContainText("green on Linux");
  await expect(row.getByRole("button", { name: "Clear goal" })).toBeVisible();
  await expect(row.getByRole("button", { name: "Pause goal" })).toBeVisible();
  await expect(row.getByRole("button", { name: "Edit goal" })).toBeVisible();

  await expect(row).not.toContainText("$4.50/$5.00");
  await expect(row).not.toContainText("7/20");
  await expect(row).not.toContainText("31");
  await expect(row.locator("[role=progressbar]")).toHaveCount(0);
});

for (const theme of ["light", "dark"] as const) {
  for (const state of VISUAL_AGENT_STATES) {
    test(`agent golden ${theme} ${state}`, async ({ page }) => {
      await page.goto(`/visual/?fixture=agent&theme=${theme}&state=${state}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (state === "long-content") {
        await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
        await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
      }
      // The canonical tool-shell frame exists to photograph the tool grammar,
      // so open its completed wave before capturing it. A collapsed "6 steps"
      // row cannot catch icon, status, grouping or preview regressions in the
      // components the state is named for.
      if (state === "tool-shells") {
        await page.getByRole("button", { name: /steps/ }).first().click();
        await page
          .locator('[data-tool="apply_patch"] button[aria-expanded]')
          .filter({ hasText: "specialisedPreviewProjections.ts" })
          .click();
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
