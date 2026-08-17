#!/usr/bin/env node
// Boot-contract guard — the pre-module bootstrap must agree with the app it boots.
//
// `index.html` runs two inline scripts before any module loads, because both jobs
// have to happen before the first paint: pick the scheme class + canvas colour,
// and mark the input modality. Neither can import anything — that's the point of
// being inline — so each one restates something another file owns:
//
//   - the localStorage key the preference store persists under,
//   - the class names the theme painter writes,
//   - the canvas colour per scheme, which lives in globals.css as a token,
//   - the attribute the focus-ring rule gates on,
//   - and, across the language boundary, the native window colour in main.go,
//     which shows for the frame before the WebView paints.
//
// Contracts in three languages, held by nothing. One had already drifted: the dark
// canvas was #121212 against a #101010 token, so a dark cold boot painted one grey and
// repainted another a moment later. This pins each of them.
//
// The same shape recurs one layer in, so the file grew past the bootstrap itself: any
// value a running app derives and globals.css also states as a literal, plus the motion
// ladder, which three files state independently. No count here on purpose — it was
// written as "five", and was wrong by the time anyone read it.
//
// Every one of them fails silently when it drifts — the wrong scheme for a frame,
// a ring that never shows, a flash of the wrong grey — which is exactly the class
// of defect nobody files a bug for.

import { readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(path, import.meta.url).pathname, "utf8");

const html = read("../index.html");
const css = read("../src/styles/globals.css");
const store = read("../src/state/uiStore.ts");
const painter = read("../src/plugins/builtin/theme/adapters/documentAppearance.ts");
const shell = read("../../main.go");
const typography = read("../src/lib/typography.ts");

const failures = [];

function expect(condition, message) {
  if (!condition) failures.push(message);
}

// ── 1. The persisted-preference key ───────────────────────────────────────────
const htmlStorageKey = html.match(/localStorage\.getItem\("([^"]+)"\)/)?.[1];
const storeName = store.match(/name:\s*"([^"]+)"/)?.[1];
expect(
  htmlStorageKey !== undefined && htmlStorageKey === storeName,
  `index.html reads localStorage "${htmlStorageKey}" but the store persists under "${storeName}"`,
);

// ── 2. The scheme class names ─────────────────────────────────────────────────
const htmlSchemeClass = html.match(/classList\.add\("([a-z]+)-"\s*\+\s*scheme\)/)?.[1];
expect(
  htmlSchemeClass !== undefined &&
    painter.includes(`\`${htmlSchemeClass}-\${scheme}\``) &&
    css.includes(`.${htmlSchemeClass}-dark`),
  `index.html writes "${htmlSchemeClass}-{scheme}", which the painter or globals.css does not use`,
);

// ── 3. The canvas colour per scheme ───────────────────────────────────────────
// globals.css declares --color-bg twice: light scheme first, then dark.
const canvasTokens = [...css.matchAll(/--color-bg:\s*(#[0-9a-fA-F]{3,8})/g)].map((m) =>
  m[1].toLowerCase(),
);
const htmlCanvas = html
  .match(
    /backgroundColor\s*=\s*scheme === "light" \? "(#[0-9a-fA-F]{3,8})" : "(#[0-9a-fA-F]{3,8})"/,
  )
  ?.slice(1, 3)
  .map((hex) => hex.toLowerCase());
expect(
  canvasTokens.length === 2,
  `globals.css declares --color-bg ${canvasTokens.length} time(s); expected one per scheme`,
);
expect(
  htmlCanvas !== undefined &&
    canvasTokens.length === 2 &&
    htmlCanvas.join() === canvasTokens.join(),
  `index.html paints ${htmlCanvas?.join(" / ")} where --color-bg is ${canvasTokens.join(" / ")}`,
);

// ── 4. The input-modality attribute ───────────────────────────────────────────
const modalityAttr = html.match(/setAttribute\("(data-[a-z-]+)",\s*""\)/)?.[1];
expect(
  modalityAttr !== undefined && css.includes(`html:not([${modalityAttr}])`),
  `index.html marks [${modalityAttr}] but no globals.css rule gates on it`,
);

// ── 5. The native window colour ───────────────────────────────────────────────
// The window opens before the WebView paints, so its colour must be the LIGHT
// canvas: the shell can't read a preference the WebView owns, and light is the
// app's default scheme. A dark-theme launch shows this frame briefly — the cost
// of not giving Go a second copy of the theme to read.
// v3 spells this `application.NewRGB(r, g, b)` on the WINDOW's options, where v2 had a
// `&options.RGBA{...}` literal on the application's. Matching the constructor rather than
// a struct literal is also why this reads as a call: there is no field to name.
const rgba = shell.match(
  /BackgroundColour:\s*application\.NewRGB\(\s*(\d+),\s*(\d+),\s*(\d+)\s*\)/,
);
const shellHex =
  rgba &&
  `#${[rgba[1], rgba[2], rgba[3]].map((v) => Number(v).toString(16).padStart(2, "0")).join("")}`;
expect(
  shellHex !== null && canvasTokens.length === 2 && shellHex === canvasTokens[0],
  `main.go opens the window ${shellHex} where the light --color-bg is ${canvasTokens[0]}`,
);

// ── 6. The first-paint values of every runtime-derived ladder ─────────────────
// globals.css restates the type ladder and the surface-depth step as literals so
// the pre-hydration frame has something to draw with. Both are then recomputed
// from a store default the moment JS runs, and neither restatement is checked by
// anything — so `--depth-step` had drifted to 4% against a store default of 60,
// which resolves to 6.8%. Every recessed fill was painted one value and repainted
// 70% deeper, and the deeper one put semantic text below 4.5:1 contrast. Same
// class of silent drift as the canvas colour above, one layer in.
const typeDefault = Number(
  typography.match(/UI_FONT_SIZE_DEFAULT_PX\s*=\s*(\d+)/)?.[1] ?? Number.NaN,
);
const stepRatios = [...typography.matchAll(/"(ui-[\da-z]+|code)":\s*\{\s*ratio:\s*([\d.]+)/g)];
for (const [, step, ratio] of stepRatios) {
  const declared = css.match(new RegExp(`--fs-${step}:\\s*(\\d+)px`))?.[1];
  const derived = Math.round(typeDefault * Number(ratio));
  expect(
    declared !== undefined && Number(declared) === derived,
    `globals.css paints --fs-${step} at ${declared}px, but base ${typeDefault} derives ${derived}px`,
  );
}

// The step is scheme-aware — dark doubles it, because equal ink percentages do
// not buy equal separation — so globals.css restates it twice, once per scheme,
// and both restatements are checked against the one formula that owns them.
const depthFormula = painter.match(
  /const step = \(([\d.]+)\s*\+\s*\(contrast\s*\/\s*100\)\s*\*\s*([\d.]+)\)\s*\*\s*\(scheme === "dark" \? (\d+) : 1\)/,
);
const contrastDefault = Number(store.match(/contrast:\s*([\d.]+)/)?.[1] ?? Number.NaN);
const derivedDepths = depthFormula && [
  (Number(depthFormula[1]) + (contrastDefault / 100) * Number(depthFormula[2])).toFixed(1),
  (
    (Number(depthFormula[1]) + (contrastDefault / 100) * Number(depthFormula[2])) *
    Number(depthFormula[3])
  ).toFixed(1),
];
const declaredDepths = [...css.matchAll(/--depth-step:\s*([\d.]+)%/g)].map((m) => m[1]);
expect(
  derivedDepths !== null &&
    declaredDepths.length === 2 &&
    declaredDepths.every((declared, i) => Number(declared) === Number(derivedDepths[i])),
  `globals.css paints --depth-step at ${declaredDepths.join(" / ")}%, but contrast ${contrastDefault} derives ${derivedDepths?.join(" / ")}%`,
);

// ── 7. The motion ladder, in all three places that state it ──────────────────
// The shipped visual style owns these numbers. Two other files restate them: the
// fallback every consumer stands on before a style publishes, and globals.css, which
// paints the frames before hydration. Neither restatement was checked by anything, and
// the fallback's own doc comment records `drawerMs` having drifted to 300 against a
// style shipping 240 — a drawer that travelled on one clock cold and another warm.
// Exactly the class of defect this file exists for: nothing errors, the motion is
// simply wrong for the first frames and nobody can say why.
const strip = (src) => src.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");
const styleMotion = strip(read("../src/plugins/builtin/theme/visualStyles/tokens.ts"));
const fallbackMotion = strip(read("../src/lib/appearance.ts"));

const MIRRORED_DURATIONS = {
  instantMs: "instant",
  fastMs: "fast",
  mediumMs: "med",
  disclosureMs: "disclosure",
  slowMs: "slow",
  drawerMs: "drawer",
};
for (const [field, token] of Object.entries(MIRRORED_DURATIONS)) {
  const owned = styleMotion.match(new RegExp(`\\b${field}:\\s*(\\d+)`))?.[1];
  const fallback = fallbackMotion.match(new RegExp(`\\b${field}:\\s*(\\d+)`))?.[1];
  const painted = css.match(new RegExp(`--dur-${token}-base:\\s*(\\d+)ms`))?.[1];
  expect(
    owned !== undefined && fallback === owned && painted === owned,
    `${field} is ${owned} in the shipped style, ${fallback} in the appearance fallback, ` +
      `and ${painted} in globals.css --dur-${token}-base`,
  );
}

// The drawer's sampled spring travels with its duration and is mirrored the same three
// ways. Keep the comparison textual: a missing or reordered stop changes velocity even
// when the endpoints still happen to be zero and one.
const drawerProgressOf = (src) =>
  src
    .match(/\bdrawerProgress:\s*\[([^\]]+)\]/)?.[1]
    .replace(/\s+/g, " ")
    .replace(/,\s*$/, "")
    .trim();
const paintedDrawerProgress = css
  .match(/--ease-drawer:\s*linear\(([^)]+)\)/)?.[1]
  .replace(/\s+/g, " ")
  .trim();
expect(
  drawerProgressOf(styleMotion) !== undefined &&
    drawerProgressOf(fallbackMotion) === drawerProgressOf(styleMotion) &&
    paintedDrawerProgress === drawerProgressOf(styleMotion),
  `drawerProgress is [${drawerProgressOf(styleMotion)}] in the shipped style, ` +
    `[${drawerProgressOf(fallbackMotion)}] in the appearance fallback, and ` +
    `linear(${paintedDrawerProgress}) in globals.css`,
);

if (failures.length > 0) {
  console.error(`[check-bootstrap] ${failures.length} boot-contract drift(s):\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log("[check-bootstrap] OK — the pre-paint bootstrap agrees with the app");
